package connect

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect/v2"
	"connectrpc.com/connect/v2/connecthttp"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"
	"github.com/pj-hoakari/internal-jwt-handling/interceptor"
	"github.com/pj-hoakari/internal-jwt-handling/jwks"
	"github.com/pj-hoakari/internal-jwt-handling/jwtgen"
	"github.com/pj-hoakari/internal-jwt-handling/verifier"

	operationv1 "github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1"
	"github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1/operationv1connect"
	"github.com/pj-hoakari/tolo-operation/internal/application"
)

// newTestJWKSURL serves keys from an httptest endpoint, mirroring the Service
// Gateway publishing its signing keys, and returns the URL to fetch them from.
func newTestJWKSURL(t *testing.T, keys internaljwt.JWKS) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(keys); err != nil {
			t.Errorf("encode JWKS: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	return server.URL
}

// newTestVerifier builds a verifier backed by an httptest JWKS endpoint serving
// keys.
func newTestVerifier(t *testing.T, keys internaljwt.JWKS) *verifier.Verifier {
	t.Helper()

	// The cooldowns are collapsed so that a refresh on an unknown kid is never
	// held off within a test run.
	cache, err := jwks.New(jwks.Config{
		URL:             newTestJWKSURL(t, keys),
		RefreshCooldown: time.Nanosecond,
		FailureCooldown: time.Nanosecond,
	})
	if err != nil {
		t.Fatalf("create JWKS cache: %v", err)
	}

	tokenVerifier, err := verifier.New(DefaultInternalJWTIssuer, DefaultInternalJWTAudience, cache)
	if err != nil {
		t.Fatalf("create internal JWT verifier: %v", err)
	}

	return tokenVerifier
}

type testUseCases struct {
	staffCommunication application.StaffCommunicationUseCases
}

func (u testUseCases) routes(t *testing.T, tokenVerifier interceptor.TokenVerifier) (func(*http.ServeMux), error) {
	t.Helper()

	if u.staffCommunication == nil {
		u.staffCommunication = &fakeStaffCommunication{}
	}

	return RoutesWithVerifier(tokenVerifier, u.staffCommunication)
}

// newTestHandler builds a handler serving the production service routes wired
// to a verifier trusting keys.
func newTestHandler(t *testing.T, keys internaljwt.JWKS) http.Handler {
	t.Helper()

	return newTestHandlerWith(t, keys, testUseCases{})
}

func newTestHandlerWith(t *testing.T, keys internaljwt.JWKS, useCases testUseCases) http.Handler {
	t.Helper()

	routes, err := useCases.routes(t, newTestVerifier(t, keys))
	if err != nil {
		t.Fatalf("RoutesWithVerifier() error = %v", err)
	}

	mux := http.NewServeMux()
	routes(mux)

	return mux
}

func newTestHandlerForJWKSURL(t *testing.T, jwksURL string) http.Handler {
	t.Helper()

	cache, err := jwks.New(jwks.Config{
		URL:             jwksURL,
		RefreshCooldown: time.Nanosecond,
		FailureCooldown: time.Nanosecond,
		RetryBackoff:    []time.Duration{},
	})
	if err != nil {
		t.Fatalf("create JWKS cache: %v", err)
	}

	tokenVerifier, err := verifier.New(DefaultInternalJWTIssuer, DefaultInternalJWTAudience, cache)
	if err != nil {
		t.Fatalf("create internal JWT verifier: %v", err)
	}

	routes, err := testUseCases{}.routes(t, tokenVerifier)
	if err != nil {
		t.Fatalf("RoutesWithVerifier() error = %v", err)
	}

	mux := http.NewServeMux()
	routes(mux)

	return mux
}

const (
	testTenantPublicID = "a1b2c3d4e5f60718"
	testEventPublicID  = "0f1e2d3c4b5a6978"
	testSubject        = "staff-user-1"
)

func mintEventToken(t *testing.T, scope string) (string, internaljwt.JWKS) {
	t.Helper()

	return mintInternalJWTFor(t, jwtgen.Config{
		TokenUse:       internaljwt.TokenUseEventAccess,
		Subject:        testSubject,
		TenantPublicID: testTenantPublicID,
		EventPublicID:  testEventPublicID,
		Scope:          scope,
	})
}

// mintInternalJWTFor issues an internal JWT signed by a fresh key, returning
// the Authorization header value and the JWKS document publishing the key.
func mintInternalJWTFor(t *testing.T, config jwtgen.Config) (string, internaljwt.JWKS) {
	t.Helper()

	if config.Issuer == "" {
		config.Issuer = DefaultInternalJWTIssuer
	}

	if config.Audience == "" {
		config.Audience = DefaultInternalJWTAudience
	}

	config.KeyID = "test-key"
	config.TTL = time.Hour

	output, err := jwtgen.Generate(config)
	if err != nil {
		t.Fatalf("generate internal JWT: %v", err)
	}

	return "Bearer " + output.Token, output.JWKS
}

func authorized(authorization string) context.Context {
	ctx, info := connectrpc.NewClientContext(context.Background())
	if authorization != "" {
		info.RequestHeader().Set("Authorization", authorization)
	}

	return ctx
}

func newTestServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	httpServer := httptest.NewServer(handler)
	t.Cleanup(httpServer.Close)

	return httpServer
}

func newClient(httpServer *httptest.Server) *connectrpc.Client {
	return connectrpc.NewClient(connecthttp.NewTransport(httpServer.Client(), httpServer.URL))
}

func getHistoryDigest(httpServer *httptest.Server, authorization string) error {
	client := operationv1connect.NewDigestServiceClient(newClient(httpServer))
	_, err := client.GetHistoryDigest(authorized(authorization), &operationv1.GetHistoryDigestRequest{EventId: testEventPublicID})

	return err
}

func TestRoutesWithJWTSettings(t *testing.T) {
	t.Parallel()

	t.Run("verifies a token against the JWKS the settings locate", func(t *testing.T) {
		t.Parallel()

		authorization, keys := mintEventToken(t, "events.read")

		settings := DefaultJWTSettings()
		settings.JWKSURL = newTestJWKSURL(t, keys)

		routes, err := RoutesWithJWTSettings(settings, &fakeStaffCommunication{})
		if err != nil {
			t.Fatalf("RoutesWithJWTSettings() error = %v", err)
		}

		mux := http.NewServeMux()
		routes(mux)

		err = getHistoryDigest(newTestServer(t, mux), authorization)
		if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnimplemented; got != want {
			t.Fatalf("GetHistoryDigest() error code = %v, want %v", got, want)
		}
	})

	t.Run("rejects settings without a JWKS URL", func(t *testing.T) {
		t.Parallel()

		settings := DefaultJWTSettings()
		settings.JWKSURL = ""

		_, err := RoutesWithJWTSettings(settings, &fakeStaffCommunication{})
		if !errors.Is(err, jwks.ErrMissingURL) {
			t.Fatalf("RoutesWithJWTSettings() error = %v, want %v", err, jwks.ErrMissingURL)
		}
	})
}

func TestEventAccessAuthz(t *testing.T) {
	t.Parallel()

	eventToken := jwtgen.Config{
		TokenUse:       internaljwt.TokenUseEventAccess,
		Subject:        testSubject,
		TenantPublicID: testTenantPublicID,
		EventPublicID:  testEventPublicID,
		Scope:          "events.read",
	}

	tests := []struct {
		name   string
		config func(jwtgen.Config) jwtgen.Config
		want   connectrpc.Code
	}{
		{"accepts an event token with the required scope", func(c jwtgen.Config) jwtgen.Config { return c }, connectrpc.CodeUnimplemented},
		{"rejects an event token without the required scope", func(c jwtgen.Config) jwtgen.Config {
			c.Scope = "events.operate"

			return c
		}, connectrpc.CodePermissionDenied},
		{"rejects a tenant token", func(c jwtgen.Config) jwtgen.Config {
			c.TokenUse = internaljwt.TokenUseTenantAccess
			c.EventPublicID = ""

			return c
		}, connectrpc.CodeUnauthenticated},
		{"rejects a service token", func(c jwtgen.Config) jwtgen.Config {
			return jwtgen.Config{TokenUse: internaljwt.TokenUseService, Subject: "tolo-observation"}
		}, connectrpc.CodeUnauthenticated},
		{"rejects another audience", func(c jwtgen.Config) jwtgen.Config {
			c.Audience = "other-service"

			return c
		}, connectrpc.CodeUnauthenticated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			authorization, keys := mintInternalJWTFor(t, tt.config(eventToken))

			err := getHistoryDigest(newTestServer(t, newTestHandler(t, keys)), authorization)
			if got := connectrpc.CodeOf(err); got != tt.want {
				t.Fatalf("GetHistoryDigest() error code = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthzRejectsMissingBearerToken(t *testing.T) {
	t.Parallel()

	_, keys := mintEventToken(t, "events.read")

	err := getHistoryDigest(newTestServer(t, newTestHandler(t, keys)), "")
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
		t.Fatalf("GetHistoryDigest() error code = %v, want %v", got, want)
	}
}

func TestAuthzRejectsUnknownSigningKey(t *testing.T) {
	t.Parallel()

	// The handler trusts a JWKS publishing neither the key nor the kid that
	// signed the token below.
	_, trustedKeys := mintEventToken(t, "events.read")
	for i := range trustedKeys.Keys {
		trustedKeys.Keys[i].KeyID = "other-key"
	}

	foreignAuthorization, _ := mintEventToken(t, "events.read")

	err := getHistoryDigest(newTestServer(t, newTestHandler(t, trustedKeys)), foreignAuthorization)
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
		t.Fatalf("GetHistoryDigest() error code = %v, want %v", got, want)
	}
}

func TestAuthzUnavailableWhenJWKSUnreachable(t *testing.T) {
	t.Parallel()

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(jwksServer.Close)

	authorization, _ := mintEventToken(t, "events.read")

	err := getHistoryDigest(newTestServer(t, newTestHandlerForJWKSURL(t, jwksServer.URL)), authorization)
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnavailable; got != want {
		t.Fatalf("GetHistoryDigest() error code = %v, want %v", got, want)
	}
}

func TestInternalProcedureAcceptsOnlyServiceTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config jwtgen.Config
		want   connectrpc.Code
	}{
		{"accepts a service token", jwtgen.Config{TokenUse: internaljwt.TokenUseService, Subject: "tolo-observation"}, connectrpc.CodeUnimplemented},
		{"rejects an event token", jwtgen.Config{
			TokenUse:       internaljwt.TokenUseEventAccess,
			Subject:        testSubject,
			TenantPublicID: testTenantPublicID,
			EventPublicID:  testEventPublicID,
			Scope:          "events.operate events.read",
		}, connectrpc.CodeUnauthenticated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			authorization, keys := mintInternalJWTFor(t, tt.config)
			client := operationv1connect.NewDeliveryCoordinationServiceClient(newClient(newTestServer(t, newTestHandler(t, keys))))

			_, err := client.RequestProposalDelivery(authorized(authorization), &operationv1.RequestProposalDeliveryRequest{EventId: testEventPublicID})
			if got := connectrpc.CodeOf(err); got != tt.want {
				t.Fatalf("RequestProposalDelivery() error code = %v, want %v", got, tt.want)
			}
		})
	}
}

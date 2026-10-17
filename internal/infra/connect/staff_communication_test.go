package connect

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	connectrpc "connectrpc.com/connect/v2"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"
	"github.com/pj-hoakari/internal-jwt-handling/jwtgen"

	operationv1 "github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1"
	"github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1/operationv1connect"
	"github.com/pj-hoakari/tolo-operation/internal/application"
	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
	"github.com/pj-hoakari/tolo-operation/internal/tenantctx"
)

const testMessageID = "1122334455667788"

var testSentAt = time.Date(2026, time.October, 17, 9, 0, 0, 0, time.UTC)

type fakeStaffCommunication struct {
	sendInput   application.SendStaffMessageInput
	revokeInput application.RevokeStaffMessageInput
	shareInput  application.ShareGuidanceInfoInput
	listInput   application.ListStaffMessagesInput
	message     domain.StaffMessage
	messages    []domain.StaffMessage
	err         error
}

func (f *fakeStaffCommunication) SendStaffMessage(_ context.Context, input application.SendStaffMessageInput) (domain.StaffMessage, error) {
	f.sendInput = input

	return f.message, f.err
}

func (f *fakeStaffCommunication) RevokeStaffMessage(_ context.Context, input application.RevokeStaffMessageInput) error {
	f.revokeInput = input

	return f.err
}

func (f *fakeStaffCommunication) ShareGuidanceInfo(_ context.Context, input application.ShareGuidanceInfoInput) (domain.StaffMessage, error) {
	f.shareInput = input

	return f.message, f.err
}

func (f *fakeStaffCommunication) ListStaffMessages(_ context.Context, input application.ListStaffMessagesInput) ([]domain.StaffMessage, error) {
	f.listInput = input

	return f.messages, f.err
}

func newStaffCommunicationClient(
	t *testing.T,
	config jwtgen.Config,
	useCases *fakeStaffCommunication,
) (operationv1connect.StaffCommunicationServiceClient, string) {
	t.Helper()

	authorization, keys := mintInternalJWTFor(t, config)
	httpServer := newTestServer(t, newTestHandlerWith(t, keys, testUseCases{staffCommunication: useCases}))

	return operationv1connect.NewStaffCommunicationServiceClient(newClient(httpServer)), authorization
}

func staffToken(scope string) jwtgen.Config {
	return jwtgen.Config{
		TokenUse:       internaljwt.TokenUseEventAccess,
		Subject:        testSubject,
		TenantPublicID: testTenantPublicID,
		EventPublicID:  testEventPublicID,
		Scope:          scope,
	}
}

func TestSendStaffMessage(t *testing.T) {
	t.Parallel()

	useCases := &fakeStaffCommunication{message: domain.StaffMessage{
		ID:           testMessageID,
		EventID:      testEventPublicID,
		SenderUserID: testSubject,
		Kind:         domain.MessageKindStaffMessage,
		Audience:     domain.AudienceGuest,
		Body:         "gate B is open",
		SentAt:       testSentAt,
	}}
	client, authorization := newStaffCommunicationClient(t, staffToken("events.operate"), useCases)

	res, err := client.SendStaffMessage(authorized(authorization), &operationv1.SendStaffMessageRequest{
		EventId:  testEventPublicID,
		Audience: operationv1.Audience_AUDIENCE_GUEST,
		Body:     "gate B is open",
	})
	if err != nil {
		t.Fatalf("SendStaffMessage: %v", err)
	}

	wantInput := application.SendStaffMessageInput{EventID: testEventPublicID, Audience: domain.AudienceGuest, Body: "gate B is open"}
	if useCases.sendInput != wantInput {
		t.Errorf("input = %+v, want %+v", useCases.sendInput, wantInput)
	}

	if res.GetMessageId() != testMessageID || res.GetEventId() != testEventPublicID || res.GetSenderUserId() != testSubject ||
		res.GetAudience() != operationv1.Audience_AUDIENCE_GUEST || res.GetBody() != "gate B is open" ||
		!res.GetSentAt().AsTime().Equal(testSentAt) {
		t.Errorf("SendStaffMessage = %v", res)
	}
}

func TestShareGuidanceInfo(t *testing.T) {
	t.Parallel()

	useCases := &fakeStaffCommunication{message: domain.StaffMessage{ID: testMessageID, Audience: domain.AudienceStaff}}
	client, authorization := newStaffCommunicationClient(t, staffToken("events.operate"), useCases)

	res, err := client.ShareGuidanceInfo(authorized(authorization), &operationv1.ShareGuidanceInfoRequest{
		EventId:         testEventPublicID,
		Body:            "one way",
		RelatedRouteIds: []string{"route-a"},
	})
	if err != nil {
		t.Fatalf("ShareGuidanceInfo: %v", err)
	}

	if useCases.shareInput.Body != "one way" || !slices.Equal(useCases.shareInput.RelatedRouteIDs, []string{"route-a"}) {
		t.Errorf("input = %+v", useCases.shareInput)
	}

	if res.GetAudience() != operationv1.Audience_AUDIENCE_STAFF {
		t.Errorf("audience = %v, want %v", res.GetAudience(), operationv1.Audience_AUDIENCE_STAFF)
	}
}

func TestListStaffMessages(t *testing.T) {
	t.Parallel()

	for name, config := range map[string]jwtgen.Config{
		"event token":   staffToken("events.read"),
		"service token": {TokenUse: internaljwt.TokenUseService, Subject: "tolo-guest"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			useCases := &fakeStaffCommunication{messages: []domain.StaffMessage{
				{ID: "1111111111111111", Audience: domain.AudienceGuest},
				{ID: "2222222222222222", Audience: domain.AudienceBoth},
			}}
			client, authorization := newStaffCommunicationClient(t, config, useCases)

			res, err := client.ListStaffMessages(authorized(authorization), &operationv1.ListStaffMessagesRequest{
				EventId:  testEventPublicID,
				Audience: operationv1.Audience_AUDIENCE_GUEST,
			})
			if err != nil {
				t.Fatalf("ListStaffMessages: %v", err)
			}

			if useCases.listInput.Audience != domain.AudienceGuest {
				t.Errorf("audience input = %q, want %q", useCases.listInput.Audience, domain.AudienceGuest)
			}

			got := make([]string, 0, len(res.GetMessages()))
			for _, message := range res.GetMessages() {
				got = append(got, message.GetMessageId()+"/"+message.GetAudience().String())
			}

			if want := []string{"1111111111111111/AUDIENCE_GUEST", "2222222222222222/AUDIENCE_BOTH"}; !slices.Equal(got, want) {
				t.Errorf("ListStaffMessages = %v, want %v", got, want)
			}
		})
	}
}

func TestStaffCommunicationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		code connectrpc.Code
	}{
		{fmt.Errorf("revoke staff message: %w", repository.ErrStaffMessageNotFound), connectrpc.CodeNotFound},
		{domain.ErrInvalidPublicID, connectrpc.CodeInvalidArgument},
		{tenantctx.ErrEventMismatch, connectrpc.CodePermissionDenied},
		{errors.New("connection reset"), connectrpc.CodeInternal},
	}

	for _, tt := range tests {
		t.Run(tt.code.String(), func(t *testing.T) {
			t.Parallel()

			client, authorization := newStaffCommunicationClient(t, staffToken("events.operate"), &fakeStaffCommunication{err: tt.err})

			_, err := client.RevokeStaffMessage(authorized(authorization), &operationv1.RevokeStaffMessageRequest{
				EventId:   testEventPublicID,
				MessageId: testMessageID,
			})
			if got := connectrpc.CodeOf(err); got != tt.code {
				t.Errorf("RevokeStaffMessage error code = %v, want %v", got, tt.code)
			}
		})
	}
}

func TestStaffCommunicationRejectsInvalidMessages(t *testing.T) {
	t.Parallel()

	for _, err := range []error{domain.ErrAudienceRequired, domain.ErrBodyRequired, domain.ErrBodyTooLong, domain.ErrRouteIDRequired} {
		client, authorization := newStaffCommunicationClient(t, staffToken("events.operate"), &fakeStaffCommunication{err: err})

		_, got := client.SendStaffMessage(authorized(authorization), &operationv1.SendStaffMessageRequest{EventId: testEventPublicID})
		if connectrpc.CodeOf(got) != connectrpc.CodeInvalidArgument {
			t.Errorf("%v: SendStaffMessage error code = %v, want %v", err, connectrpc.CodeOf(got), connectrpc.CodeInvalidArgument)
		}
	}
}

func TestListStaffMessagesAsAServiceForAnotherAudienceIsDenied(t *testing.T) {
	t.Parallel()

	client, authorization := newStaffCommunicationClient(
		t,
		jwtgen.Config{TokenUse: internaljwt.TokenUseService, Subject: "tolo-guest"},
		&fakeStaffCommunication{err: application.ErrGuestAudienceOnly},
	)

	_, err := client.ListStaffMessages(authorized(authorization), &operationv1.ListStaffMessagesRequest{EventId: testEventPublicID})
	if got, want := connectrpc.CodeOf(err), connectrpc.CodePermissionDenied; got != want {
		t.Errorf("ListStaffMessages error code = %v, want %v", got, want)
	}
}

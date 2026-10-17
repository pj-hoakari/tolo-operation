package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"

	"github.com/pj-hoakari/tolo-operation/internal/application"
	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/tenantctx"
)

func newDeliveryCoordination(t *testing.T) (*application.DeliveryCoordinationService, *MockConnectionStateRepository, *MockFeedbackRepository) {
	t.Helper()

	controller := gomock.NewController(t)
	states := NewMockConnectionStateRepository(controller)
	feedback := NewMockFeedbackRepository(controller)

	return application.NewDeliveryCoordinationService(states, feedback), states, feedback
}

func TestUpdateConnectionStateSavesTheCallersDeviceInTheRequestEvent(t *testing.T) {
	t.Parallel()

	service, states, _ := newDeliveryCoordination(t)
	states.EXPECT().Save(gomock.Any(), domain.ConnectionState{
		TenantPublicID: testTenantPublicID,
		EventID:        testEventPublicID,
		UserID:         testSubject,
		DeviceID:       "phone-1",
		Connected:      true,
		At:             testNow,
	}).Return(nil)

	err := service.UpdateConnectionState(withEventToken(testEventPublicID), application.UpdateConnectionStateInput{
		TenantID:  " " + testTenantPublicID + " ",
		EventID:   " " + testEventPublicID + " ",
		UserID:    testSubject,
		DeviceID:  "phone-1",
		Connected: true,
		At:        testNow,
	})
	if err != nil {
		t.Fatalf("UpdateConnectionState: %v", err)
	}
}

func TestUpdateConnectionStateRejectsAnotherUser(t *testing.T) {
	t.Parallel()

	service, _, _ := newDeliveryCoordination(t)

	err := service.UpdateConnectionState(withEventToken(testEventPublicID), application.UpdateConnectionStateInput{
		TenantID: testTenantPublicID,
		EventID:  testEventPublicID,
		UserID:   "someone-else",
		DeviceID: "phone-1",
		At:       testNow,
	})
	if !errors.Is(err, application.ErrForeignUser) {
		t.Errorf("UpdateConnectionState error = %v, want %v", err, application.ErrForeignUser)
	}
}

func TestUpdateConnectionStateRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input application.UpdateConnectionStateInput
		want  error
	}{
		{"missing device", connectionState(func(input *application.UpdateConnectionStateInput) { input.DeviceID = "" }), domain.ErrDeviceRequired},
		{"missing time", connectionState(func(input *application.UpdateConnectionStateInput) { input.At = time.Time{} }), domain.ErrStateTimeRequired},
		{"missing tenant", connectionState(func(input *application.UpdateConnectionStateInput) { input.TenantID = " " }), domain.ErrTenantRequired},
		{"missing event", connectionState(func(input *application.UpdateConnectionStateInput) { input.EventID = " " }), domain.ErrEventRequired},
	}

	for _, tt := range tests {
		service, _, _ := newDeliveryCoordination(t)

		if err := service.UpdateConnectionState(withEventToken(testEventPublicID), tt.input); !errors.Is(err, tt.want) {
			t.Errorf("%s: UpdateConnectionState error = %v, want %v", tt.name, err, tt.want)
		}
	}
}

func TestUpdateConnectionStateRejectsAScopeOtherThanTheTokens(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input application.UpdateConnectionStateInput
		want  error
	}{
		{"another tenant", connectionState(func(input *application.UpdateConnectionStateInput) { input.TenantID = otherTenantPublicID }), tenantctx.ErrMismatch},
		{"another event", connectionState(func(input *application.UpdateConnectionStateInput) { input.EventID = otherEventPublicID }), tenantctx.ErrEventMismatch},
	}

	for _, tt := range tests {
		service, _, _ := newDeliveryCoordination(t)

		if err := service.UpdateConnectionState(withEventToken(testEventPublicID), tt.input); !errors.Is(err, tt.want) {
			t.Errorf("%s: UpdateConnectionState error = %v, want %v", tt.name, err, tt.want)
		}
	}
}

func connectionState(modify func(*application.UpdateConnectionStateInput)) application.UpdateConnectionStateInput {
	input := application.UpdateConnectionStateInput{
		TenantID:  testTenantPublicID,
		EventID:   testEventPublicID,
		UserID:    testSubject,
		DeviceID:  "phone-1",
		Connected: true,
		At:        testNow,
	}
	modify(&input)

	return input
}

func TestRecordFeedbackValues(t *testing.T) {
	t.Parallel()

	service, _, feedback := newDeliveryCoordination(t)
	entries := []domain.FeedbackEntry{{AttributeTag: "", Kind: "objective", Value: 0.5}}
	feedback.EXPECT().Record(gomock.Any(), domain.FeedbackValues{
		TenantPublicID: testTenantPublicID,
		EventID:        testEventPublicID,
		ComputedAt:     testNow,
		Entries:        entries,
	}).Return(nil)

	err := service.RecordFeedbackValues(withServiceToken(), application.RecordFeedbackValuesInput{
		TenantID:   " " + testTenantPublicID + " ",
		EventID:    testEventPublicID,
		ComputedAt: testNow,
		Entries:    entries,
	})
	if err != nil {
		t.Fatalf("RecordFeedbackValues: %v", err)
	}
}

func TestRecordFeedbackValuesRejectsAServiceTokenForAnotherEvent(t *testing.T) {
	t.Parallel()

	service, _, _ := newDeliveryCoordination(t)
	ctx := internaljwt.ContextWithClaims(context.Background(), internaljwt.Claims{
		TokenUse:       internaljwt.TokenUseService,
		TenantPublicID: testTenantPublicID,
		EventPublicID:  otherEventPublicID,
	})

	err := service.RecordFeedbackValues(ctx, application.RecordFeedbackValuesInput{
		TenantID:   testTenantPublicID,
		EventID:    testEventPublicID,
		ComputedAt: testNow,
	})
	if !errors.Is(err, tenantctx.ErrEventMismatch) {
		t.Errorf("RecordFeedbackValues error = %v, want %v", err, tenantctx.ErrEventMismatch)
	}
}

func TestRecordFeedbackValuesChecksTheRequestTenant(t *testing.T) {
	t.Parallel()

	tenantToken := internaljwt.ContextWithClaims(context.Background(), internaljwt.Claims{
		TokenUse:       internaljwt.TokenUseService,
		TenantPublicID: testTenantPublicID,
	})

	tests := []struct {
		name     string
		ctx      context.Context
		tenantID string
		eventID  string
		want     error
	}{
		{"missing tenant", withServiceToken(), " ", testEventPublicID, domain.ErrTenantRequired},
		{"missing event", withServiceToken(), testTenantPublicID, " ", domain.ErrEventRequired},
		{"tenant other than the token's", tenantToken, otherTenantPublicID, testEventPublicID, tenantctx.ErrMismatch},
	}

	for _, tt := range tests {
		service, _, _ := newDeliveryCoordination(t)

		err := service.RecordFeedbackValues(tt.ctx, application.RecordFeedbackValuesInput{
			TenantID:   tt.tenantID,
			EventID:    tt.eventID,
			ComputedAt: testNow,
		})
		if !errors.Is(err, tt.want) {
			t.Errorf("%s: RecordFeedbackValues error = %v, want %v", tt.name, err, tt.want)
		}
	}
}

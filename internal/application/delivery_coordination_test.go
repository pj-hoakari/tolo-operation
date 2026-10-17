package application_test

import (
	"context"
	"errors"
	"testing"

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

func TestUpdateConnectionStateSavesTheCallersDeviceInTheTokenEvent(t *testing.T) {
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
		{"missing device", application.UpdateConnectionStateInput{UserID: testSubject, At: testNow}, domain.ErrDeviceRequired},
		{"missing time", application.UpdateConnectionStateInput{UserID: testSubject, DeviceID: "phone-1"}, domain.ErrStateTimeRequired},
	}

	for _, tt := range tests {
		service, _, _ := newDeliveryCoordination(t)

		if err := service.UpdateConnectionState(withEventToken(testEventPublicID), tt.input); !errors.Is(err, tt.want) {
			t.Errorf("%s: UpdateConnectionState error = %v, want %v", tt.name, err, tt.want)
		}
	}
}

func TestRecordFeedbackValues(t *testing.T) {
	t.Parallel()

	service, _, feedback := newDeliveryCoordination(t)
	entries := []domain.FeedbackEntry{{AttributeTag: "", Kind: "objective", Value: 0.5}}
	feedback.EXPECT().Record(gomock.Any(), domain.FeedbackValues{EventID: testEventPublicID, ComputedAt: testNow, Entries: entries}).Return(nil)

	err := service.RecordFeedbackValues(withServiceToken(), application.RecordFeedbackValuesInput{
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

	err := service.RecordFeedbackValues(ctx, application.RecordFeedbackValuesInput{EventID: testEventPublicID, ComputedAt: testNow})
	if !errors.Is(err, tenantctx.ErrEventMismatch) {
		t.Errorf("RecordFeedbackValues error = %v, want %v", err, tenantctx.ErrEventMismatch)
	}
}

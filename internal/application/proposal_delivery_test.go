package application_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/mock/gomock"

	"github.com/pj-hoakari/tolo-operation/internal/application"
	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

var errNotificationDown = errors.New("notification unavailable")

type fakeNotifier struct {
	requests []application.PushRequest
	unsent   map[string]string
	err      error
}

func (f *fakeNotifier) SendPush(_ context.Context, request application.PushRequest) ([]application.PushResult, error) {
	f.requests = append(f.requests, request)
	if f.err != nil {
		return nil, f.err
	}

	results := make([]application.PushResult, 0, len(request.RecipientUserIDs))
	for _, userID := range request.RecipientUserIDs {
		reason, failed := f.unsent[userID]
		results = append(results, application.PushResult{UserID: userID, Sent: !failed, FailureReason: reason})
	}

	return results, nil
}

type proposalDeliveryDeps struct {
	states   *MockConnectionStateRepository
	outbox   *MockOutboxRepository
	notifier *fakeNotifier
}

func newProposalDelivery(t *testing.T) (*application.DeliveryCoordinationService, proposalDeliveryDeps) {
	t.Helper()

	controller := gomock.NewController(t)
	deps := proposalDeliveryDeps{
		states:   NewMockConnectionStateRepository(controller),
		outbox:   NewMockOutboxRepository(controller),
		notifier: &fakeNotifier{},
	}

	return application.NewDeliveryCoordinationService(deps.states, NewMockFeedbackRepository(controller), deps.outbox, deps.notifier), deps
}

func state(userID, deviceID string, connected bool) domain.ConnectionState {
	return domain.ConnectionState{
		TenantPublicID: testTenantPublicID,
		EventID:        testEventPublicID,
		UserID:         userID,
		DeviceID:       deviceID,
		Connected:      connected,
		At:             testNow,
	}
}

func flowProposalInput() application.RequestProposalDeliveryInput {
	return application.RequestProposalDeliveryInput{
		TenantID:        testTenantPublicID,
		EventID:         testEventPublicID,
		ProposalKind:    domain.ProposalFlowProposals,
		ProposalContent: json.RawMessage(`{"detour_proposals":[{"origin_route_id":"route-a","route_ids":["route-b"]}]}`),
	}
}

func TestRequestProposalDeliveryRoutesRecipientsByConnectionState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		states       []domain.ConnectionState
		wantRealtime []string
		wantPush     []string
	}{
		{
			name:         "all connected",
			states:       []domain.ConnectionState{state("staff-a", "phone", true), state("staff-b", "phone", true)},
			wantRealtime: []string{"staff-a", "staff-b"},
			wantPush:     nil,
		},
		{
			name:         "all disconnected",
			states:       []domain.ConnectionState{state("staff-a", "phone", false), state("staff-b", "phone", false)},
			wantRealtime: nil,
			wantPush:     []string{"staff-a", "staff-b"},
		},
		{
			name: "mixed, with one staff member open on any of two devices",
			states: []domain.ConnectionState{
				state("staff-a", "phone", false), state("staff-a", "tablet", true),
				state("staff-b", "phone", false),
				state("staff-c", "phone", true),
			},
			wantRealtime: []string{"staff-a", "staff-c"},
			wantPush:     []string{"staff-b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service, deps := newProposalDelivery(t)
			deps.states.EXPECT().ListByEvent(gomock.Any(), testTenantPublicID, testEventPublicID).Return(tt.states, nil)

			var enqueued []domain.OutboxMessage

			if tt.wantRealtime != nil {
				deps.outbox.EXPECT().Enqueue(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, message domain.OutboxMessage) error {
					enqueued = append(enqueued, message)

					return nil
				})
			}

			result, err := service.RequestProposalDelivery(withServiceToken(), flowProposalInput())
			if err != nil {
				t.Fatalf("RequestProposalDelivery: %v", err)
			}

			id, err := uuid.Parse(string(result.DeliveryID))
			if err != nil || id.Version() != 7 {
				t.Errorf("delivery ID %q, want a UUIDv7", result.DeliveryID)
			}

			if result.RealtimeCount != len(tt.wantRealtime) || result.NotificationCount != len(tt.wantPush) {
				t.Errorf("counts = realtime %d, notification %d, want %d and %d",
					result.RealtimeCount, result.NotificationCount, len(tt.wantRealtime), len(tt.wantPush))
			}

			if tt.wantRealtime != nil {
				assertDeliveryOrder(t, enqueued, result.DeliveryID, tt.wantRealtime)
			}

			if tt.wantPush == nil {
				if len(deps.notifier.requests) != 0 {
					t.Errorf("SendPush called %d times, want none", len(deps.notifier.requests))
				}

				return
			}

			if len(deps.notifier.requests) != 1 {
				t.Fatalf("SendPush called %d times, want once", len(deps.notifier.requests))
			}

			request := deps.notifier.requests[0]
			if !reflect.DeepEqual(request.RecipientUserIDs, tt.wantPush) || request.EventID != testEventPublicID ||
				request.Message.Data["delivery_id"] != string(result.DeliveryID) || request.Message.Data["kind"] != "flow_proposals" {
				t.Errorf("SendPush request = %+v, want %v for delivery %s", request, tt.wantPush, result.DeliveryID)
			}
		})
	}
}

func assertDeliveryOrder(t *testing.T, enqueued []domain.OutboxMessage, deliveryID domain.DeliveryID, recipients []string) {
	t.Helper()

	if len(enqueued) != 1 {
		t.Fatalf("enqueued %d messages, want one delivery order", len(enqueued))
	}

	if enqueued[0].Topic != domain.TopicRealtimeDelivery || enqueued[0].OrderingKey != testEventPublicID {
		t.Errorf("enqueued topic %q key %q, want realtime-delivery keyed by the event", enqueued[0].Topic, enqueued[0].OrderingKey)
	}

	wantRecipients := make([]any, 0, len(recipients))
	for _, recipient := range recipients {
		wantRecipients = append(wantRecipients, recipient)
	}

	want := map[string]any{
		"delivery_id":        string(deliveryID),
		"event_id":           testEventPublicID,
		"recipient_user_ids": wantRecipients,
		"payload": map[string]any{"flow_proposals": map[string]any{
			"detour_proposals": []any{map[string]any{"origin_route_id": "route-a", "route_ids": []any{"route-b"}}},
		}},
	}
	if got := decodeJSON(t, enqueued[0].Payload); !reflect.DeepEqual(got, want) {
		t.Errorf("delivery order = %v, want %v", got, want)
	}
}

func TestRequestProposalDeliveryCountsOnlyPushesThatWereSent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		unsent           map[string]string
		err              error
		wantNotification int
	}{
		{"one recipient fails", map[string]string{"staff-b": "no device token"}, nil, 1},
		{"the whole call fails", nil, errNotificationDown, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service, deps := newProposalDelivery(t)
			deps.notifier.unsent, deps.notifier.err = tt.unsent, tt.err
			deps.states.EXPECT().ListByEvent(gomock.Any(), gomock.Any(), gomock.Any()).Return([]domain.ConnectionState{
				state("staff-a", "phone", false), state("staff-b", "phone", false), state("staff-c", "phone", true),
			}, nil)
			deps.outbox.EXPECT().Enqueue(gomock.Any(), gomock.Any()).Return(nil)

			result, err := service.RequestProposalDelivery(withServiceToken(), flowProposalInput())
			if err != nil {
				t.Fatalf("RequestProposalDelivery: %v, want the delivery to go on despite push failures", err)
			}

			if result.RealtimeCount != 1 || result.NotificationCount != tt.wantNotification {
				t.Errorf("counts = realtime %d, notification %d, want 1 and %d", result.RealtimeCount, result.NotificationCount, tt.wantNotification)
			}
		})
	}
}

func TestRequestProposalDeliveryToAPairWithoutStaffDeliversNowhere(t *testing.T) {
	t.Parallel()

	service, deps := newProposalDelivery(t)
	deps.states.EXPECT().ListByEvent(gomock.Any(), otherTenantPublicID, testEventPublicID).Return(nil, nil)

	input := flowProposalInput()
	input.TenantID = otherTenantPublicID

	result, err := service.RequestProposalDelivery(withServiceToken(), input)
	if err != nil {
		t.Fatalf("RequestProposalDelivery: %v", err)
	}

	if result.RealtimeCount != 0 || result.NotificationCount != 0 || len(deps.notifier.requests) != 0 {
		t.Errorf("result %+v with %d push requests, want no delivery", result, len(deps.notifier.requests))
	}
}

func TestRequestProposalDeliveryRejectsAMissingPayload(t *testing.T) {
	t.Parallel()

	service, _ := newProposalDelivery(t)
	input := flowProposalInput()
	input.ProposalKind, input.ProposalContent = "", nil

	if _, err := service.RequestProposalDelivery(withServiceToken(), input); !errors.Is(err, domain.ErrProposalRequired) {
		t.Errorf("RequestProposalDelivery error = %v, want %v", err, domain.ErrProposalRequired)
	}
}

func TestRequestProposalDeliveryFailsBeforePushingWhenTheRealtimeEnqueueFails(t *testing.T) {
	t.Parallel()

	service, deps := newProposalDelivery(t)
	deps.states.EXPECT().ListByEvent(gomock.Any(), gomock.Any(), gomock.Any()).Return([]domain.ConnectionState{
		state("staff-a", "phone", true), state("staff-b", "phone", false),
	}, nil)
	deps.outbox.EXPECT().Enqueue(gomock.Any(), gomock.Any()).Return(errors.New("database down"))

	if _, err := service.RequestProposalDelivery(withServiceToken(), flowProposalInput()); err == nil {
		t.Fatal("RequestProposalDelivery succeeded, want the enqueue failure")
	}

	if len(deps.notifier.requests) != 0 {
		t.Errorf("SendPush called %d times, want none so that a retried request does not push twice", len(deps.notifier.requests))
	}
}


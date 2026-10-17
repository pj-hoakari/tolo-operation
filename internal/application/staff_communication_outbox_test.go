package application_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/pj-hoakari/tolo-operation/internal/application"
	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

func decodeJSON(t *testing.T, raw []byte) any {
	t.Helper()

	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}

	return value
}

func TestSendStaffMessagePublishesOnlyGuestVisibleMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		audience  domain.Audience
		publishes bool
	}{
		{domain.AudienceGuest, true},
		{domain.AudienceBoth, true},
		{domain.AudienceStaff, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.audience), func(t *testing.T) {
			t.Parallel()

			service, deps := newStaffCommunicationWithOutbox(t)

			var stored domain.StaffMessage

			deps.messages.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, message domain.StaffMessage) error {
				if !deps.transactions.active {
					t.Error("Create ran outside the transaction")
				}

				stored = message

				return nil
			})

			var enqueued []domain.OutboxMessage

			if tt.publishes {
				deps.outbox.EXPECT().Enqueue(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, message domain.OutboxMessage) error {
					if !deps.transactions.active {
						t.Error("Enqueue ran outside the transaction that created the message")
					}

					enqueued = append(enqueued, message)

					return nil
				})
			}

			_, err := service.SendStaffMessage(withEventToken(testEventPublicID), application.SendStaffMessageInput{
				TenantID: testTenantPublicID,
				EventID:  testEventPublicID,
				Audience: tt.audience,
				Body:     " gate B is open ",
			})
			if err != nil {
				t.Fatalf("SendStaffMessage: %v", err)
			}

			if !tt.publishes {
				return
			}

			if len(enqueued) != 1 {
				t.Fatalf("enqueued %d messages, want 1", len(enqueued))
			}

			if got := enqueued[0]; got.Topic != domain.TopicGuestMessages || got.OrderingKey != testEventPublicID {
				t.Errorf("enqueued topic %q key %q, want guest-messages keyed by the event", got.Topic, got.OrderingKey)
			}

			want := map[string]any{
				"event_id": testEventPublicID,
				"upserts": []any{map[string]any{
					"message_id": string(stored.ID),
					"body":       "gate B is open",
					"sent_at":    "2026-10-17T09:00:00Z",
				}},
			}
			if got := decodeJSON(t, enqueued[0].Payload); !reflect.DeepEqual(got, want) {
				t.Errorf("payload = %v, want %v", got, want)
			}
		})
	}
}

func TestRevokeStaffMessagePublishesTheRevocationOnce(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		audience  domain.Audience
		revoked   bool
		publishes bool
	}{
		{"a guest message", domain.AudienceGuest, true, true},
		{"a message for both", domain.AudienceBoth, true, true},
		{"an already revoked guest message", domain.AudienceGuest, false, false},
		{"a staff message", domain.AudienceStaff, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service, deps := newStaffCommunicationWithOutbox(t)
			deps.messages.EXPECT().Revoke(gomock.Any(), testTenantPublicID, testEventPublicID, domain.MessageID(testMessageID), testNow).
				Return(tt.audience, tt.revoked, nil)

			if tt.publishes {
				deps.outbox.EXPECT().Enqueue(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, message domain.OutboxMessage) error {
					if !deps.transactions.active {
						t.Error("Enqueue ran outside the transaction that revoked the message")
					}

					want := map[string]any{"event_id": testEventPublicID, "revoked_message_ids": []any{testMessageID}}
					if message.Topic != domain.TopicGuestMessages || message.OrderingKey != testEventPublicID ||
						!reflect.DeepEqual(decodeJSON(t, message.Payload), want) {
						t.Errorf("enqueued %s %s %s, want guest-messages %v", message.Topic, message.OrderingKey, message.Payload, want)
					}

					return nil
				})
			}

			err := service.RevokeStaffMessage(withEventToken(testEventPublicID), application.RevokeStaffMessageInput{
				TenantID:  testTenantPublicID,
				EventID:   testEventPublicID,
				MessageID: testMessageID,
			})
			if err != nil {
				t.Fatalf("RevokeStaffMessage: %v", err)
			}
		})
	}
}

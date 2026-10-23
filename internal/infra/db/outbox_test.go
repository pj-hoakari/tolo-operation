package db

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"

	operationv1 "github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1"
	"github.com/pj-hoakari/tolo-operation/internal/application"
	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

var errInjected = errors.New("injected failure")

func outboxRowsFor(t *testing.T, orderingKey string) []outboxRow {
	t.Helper()

	var rows []outboxRow
	if err := testDB.SelectContext(context.Background(), &rows, `
		SELECT id, topic, ordering_key, payload, status, attempts FROM outbox_messages WHERE ordering_key = $1 ORDER BY id`,
		orderingKey); err != nil {
		t.Fatalf("select outbox rows: %v", err)
	}

	return rows
}

func pendingFor(t *testing.T, repo *PostgresOutboxRepository, orderingKey string) []domain.OutboxEntry {
	t.Helper()

	entries, err := repo.ListPending(context.Background(), 1000)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}

	var mine []domain.OutboxEntry

	for _, entry := range entries {
		if entry.Message.OrderingKey == orderingKey {
			mine = append(mine, entry)
		}
	}

	return mine
}

func TestPostgresOutboxRepositoryListsPendingInEnqueueOrderUntilSaved(t *testing.T) {
	repo := NewPostgresOutboxRepository(testDB)
	ctx := context.Background()
	key := "event-outbox-pending"

	for n := range byte(3) {
		message := domain.OutboxMessage{Topic: domain.TopicGuestMessages, OrderingKey: key, Payload: []byte{0, n + 1}}
		if err := repo.Enqueue(ctx, message); err != nil {
			t.Fatalf("Enqueue: %v", err)
		}
	}

	pending := pendingFor(t, repo, key)
	if len(pending) != 3 {
		t.Fatalf("ListPending returned %d entries, want 3", len(pending))
	}

	for i, entry := range pending {
		if want := []byte{0, byte(i + 1)}; !bytes.Equal(entry.Message.Payload, want) {
			t.Errorf("entry %d payload %x, want %x in enqueue order", i, entry.Message.Payload, want)
		}

		if entry.Status != domain.OutboxPending || entry.Attempts != 0 || entry.Message.Topic != domain.TopicGuestMessages {
			t.Errorf("entry %d = %+v, want a pending guest-messages entry without attempts", i, entry)
		}
	}

	if err := repo.Save(ctx, pending[0].Published()); err != nil {
		t.Fatalf("Save published: %v", err)
	}

	if err := repo.Save(ctx, pending[1].PublishFailed(1)); err != nil {
		t.Fatalf("Save abandoned: %v", err)
	}

	if err := repo.Save(ctx, pending[2].PublishFailed(5)); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	still := pendingFor(t, repo, key)
	if len(still) != 1 || still[0].ID != pending[2].ID || still[0].Attempts != 1 {
		t.Errorf("ListPending after saving = %+v, want only the retried entry with one attempt", still)
	}
}

type failingStaffMessages struct {
	*PostgresStaffMessageRepository
}

func (f failingStaffMessages) Create(ctx context.Context, message domain.StaffMessage) error {
	if err := f.PostgresStaffMessageRepository.Create(ctx, message); err != nil {
		return err
	}

	return errInjected
}

type failingOutbox struct {
	*PostgresOutboxRepository
}

func (f failingOutbox) Enqueue(ctx context.Context, message domain.OutboxMessage) error {
	if err := f.PostgresOutboxRepository.Enqueue(ctx, message); err != nil {
		return err
	}

	return errInjected
}

func staffContext(tenant, event string) context.Context {
	return internaljwt.ContextWithClaims(context.Background(), internaljwt.Claims{
		TokenUse:       internaljwt.TokenUseEventAccess,
		Subject:        "staff-1",
		TenantPublicID: tenant,
		EventPublicID:  event,
	})
}

func TestSendStaffMessageKeepsTheMessageAndItsOutboxEntryAtomic(t *testing.T) {
	messages := NewPostgresStaffMessageRepository(testDB)
	outbox := NewPostgresOutboxRepository(testDB)
	transactions := NewPostgresTransactor(testDB)
	clock := func() time.Time { return baseTime }

	tests := []struct {
		name     string
		service  *application.StaffCommunicationService
		event    string
		wantRows int
	}{
		{"a failed save leaves no outbox row", application.NewStaffCommunicationService(failingStaffMessages{messages}, outbox, transactions, clock), "event-outbox-failed-save", 0},
		{"a failed enqueue leaves no message", application.NewStaffCommunicationService(messages, failingOutbox{outbox}, transactions, clock), "event-outbox-failed-enqueue", 0},
		{"a successful send keeps both", application.NewStaffCommunicationService(messages, outbox, transactions, clock), "event-outbox-sent", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tenant := "tenant-outbox-atomic"

			_, err := tt.service.SendStaffMessage(staffContext(tenant, tt.event), application.SendStaffMessageInput{
				TenantID: tenant,
				EventID:  tt.event,
				Audience: domain.AudienceGuest,
				Body:     "gate B is open",
			})
			if wantErr := tt.wantRows == 0; (err != nil) != wantErr {
				t.Fatalf("SendStaffMessage error = %v, want error %v", err, wantErr)
			}

			if rows := outboxRowsFor(t, tt.event); len(rows) != tt.wantRows {
				t.Errorf("outbox rows = %d, want %d", len(rows), tt.wantRows)
			}

			active, err := messages.ListActive(context.Background(), tenant, tt.event, domain.AudiencesVisibleTo(""))
			if err != nil {
				t.Fatalf("ListActive: %v", err)
			}

			if len(active) != tt.wantRows {
				t.Errorf("stored messages = %d, want %d", len(active), tt.wantRows)
			}
		})
	}
}

func TestRevokeStaffMessageEnqueuesTheRevocationOnceAndAtomically(t *testing.T) {
	messages := NewPostgresStaffMessageRepository(testDB)
	outbox := NewPostgresOutboxRepository(testDB)
	transactions := NewPostgresTransactor(testDB)
	clock := func() time.Time { return baseTime }
	tenant, event := "tenant-outbox-revoke", "event-outbox-revoke"
	ctx := staffContext(tenant, event)
	message := createMessage(t, messages, tenant, event, domain.AudienceBoth, baseTime)
	input := application.RevokeStaffMessageInput{TenantID: tenant, EventID: event, MessageID: string(message.ID)}

	failing := application.NewStaffCommunicationService(messages, failingOutbox{outbox}, transactions, clock)
	if err := failing.RevokeStaffMessage(ctx, input); !errors.Is(err, errInjected) {
		t.Fatalf("RevokeStaffMessage with a failing outbox error = %v, want %v", err, errInjected)
	}

	active, err := messages.ListActive(context.Background(), tenant, event, domain.AudiencesVisibleTo(""))
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}

	if len(active) != 1 || len(outboxRowsFor(t, event)) != 0 {
		t.Fatalf("after a failed enqueue: %d active messages and %d outbox rows, want the message still active and no row",
			len(active), len(outboxRowsFor(t, event)))
	}

	service := application.NewStaffCommunicationService(messages, outbox, transactions, clock)
	for range 2 {
		if err := service.RevokeStaffMessage(ctx, input); err != nil {
			t.Fatalf("RevokeStaffMessage: %v", err)
		}
	}

	rows := outboxRowsFor(t, event)
	if len(rows) != 1 {
		t.Fatalf("outbox rows after revoking twice = %d, want 1", len(rows))
	}

	var payload operationv1.GuestMessagesUpdate
	if err := proto.Unmarshal(rows[0].Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	want := &operationv1.GuestMessagesUpdate{EventId: event, RevokedMessageIds: []string{string(message.ID)}}
	if !proto.Equal(&payload, want) || rows[0].Topic != string(domain.TopicGuestMessages) {
		t.Errorf("outbox row %s %v, want guest-messages %v", rows[0].Topic, &payload, want)
	}
}

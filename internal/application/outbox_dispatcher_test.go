package application_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/pj-hoakari/tolo-operation/internal/application"
	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

var errBrokerDown = errors.New("broker down")

type fakePublisher struct {
	fails     func(domain.OutboxMessage) bool
	published []domain.OutboxMessage
}

func (f *fakePublisher) Publish(_ context.Context, message domain.OutboxMessage) error {
	if f.fails != nil && f.fails(message) {
		return errBrokerDown
	}

	f.published = append(f.published, message)

	return nil
}

func pendingEntry(id int64, orderingKey string, attempts int) domain.OutboxEntry {
	return domain.OutboxEntry{
		ID: id,
		Message: domain.OutboxMessage{
			Topic:       domain.TopicGuestMessages,
			OrderingKey: orderingKey,
			Payload:     []byte(`{"event_id":"` + orderingKey + `"}`),
		},
		Status:   domain.OutboxPending,
		Attempts: attempts,
	}
}

func withState(entry domain.OutboxEntry, status domain.OutboxStatus, attempts int) domain.OutboxEntry {
	entry.Status = status
	entry.Attempts = attempts

	return entry
}

func TestDispatchPendingRetriesAFailedPublishOnTheNextPass(t *testing.T) {
	t.Parallel()

	outbox := NewMockOutboxRepository(gomock.NewController(t))
	failing := true
	publisher := &fakePublisher{fails: func(domain.OutboxMessage) bool { return failing }}
	dispatcher := application.NewOutboxDispatcher(outbox, publisher, 5)

	entry := pendingEntry(1, "event-a", 0)
	afterFailure := withState(entry, domain.OutboxPending, 1)

	gomock.InOrder(
		outbox.EXPECT().ListPending(gomock.Any(), gomock.Any()).Return([]domain.OutboxEntry{entry}, nil),
		outbox.EXPECT().Save(gomock.Any(), afterFailure).Return(nil),
		outbox.EXPECT().ListPending(gomock.Any(), gomock.Any()).Return([]domain.OutboxEntry{afterFailure}, nil),
		outbox.EXPECT().Save(gomock.Any(), withState(entry, domain.OutboxSent, 2)).Return(nil),
	)

	if err := dispatcher.DispatchPending(context.Background()); err != nil {
		t.Fatalf("first DispatchPending: %v", err)
	}

	if len(publisher.published) != 0 {
		t.Fatalf("published %d messages while the broker was down", len(publisher.published))
	}

	failing = false

	if err := dispatcher.DispatchPending(context.Background()); err != nil {
		t.Fatalf("second DispatchPending: %v", err)
	}

	if len(publisher.published) != 1 || publisher.published[0].OrderingKey != "event-a" {
		t.Errorf("published %v, want the retried entry once", publisher.published)
	}
}

func TestDispatchPendingMarksOnlyPublishedEntriesSent(t *testing.T) {
	t.Parallel()

	outbox := NewMockOutboxRepository(gomock.NewController(t))
	publisher := &fakePublisher{fails: func(m domain.OutboxMessage) bool { return m.OrderingKey == "event-a" }}
	dispatcher := application.NewOutboxDispatcher(outbox, publisher, 5)

	failed, published := pendingEntry(1, "event-a", 0), pendingEntry(2, "event-b", 0)
	outbox.EXPECT().ListPending(gomock.Any(), gomock.Any()).Return([]domain.OutboxEntry{failed, published}, nil)
	outbox.EXPECT().Save(gomock.Any(), withState(failed, domain.OutboxPending, 1)).Return(nil)
	outbox.EXPECT().Save(gomock.Any(), withState(published, domain.OutboxSent, 1)).Return(nil)

	if err := dispatcher.DispatchPending(context.Background()); err != nil {
		t.Fatalf("DispatchPending: %v", err)
	}
}

func TestDispatchPendingHoldsLaterEntriesOfAFailedOrderingKey(t *testing.T) {
	t.Parallel()

	outbox := NewMockOutboxRepository(gomock.NewController(t))
	publisher := &fakePublisher{fails: func(m domain.OutboxMessage) bool { return string(m.Payload) == `{"event_id":"event-a"}` }}
	dispatcher := application.NewOutboxDispatcher(outbox, publisher, 5)

	first := pendingEntry(1, "event-a", 0)
	held := pendingEntry(2, "event-a", 0)
	held.Message.Payload = []byte(`{"event_id":"event-a","revoked_message_ids":["x"]}`)
	other := pendingEntry(3, "event-b", 0)

	outbox.EXPECT().ListPending(gomock.Any(), gomock.Any()).Return([]domain.OutboxEntry{first, held, other}, nil)
	outbox.EXPECT().Save(gomock.Any(), withState(first, domain.OutboxPending, 1)).Return(nil)
	outbox.EXPECT().Save(gomock.Any(), withState(other, domain.OutboxSent, 1)).Return(nil)

	if err := dispatcher.DispatchPending(context.Background()); err != nil {
		t.Fatalf("DispatchPending: %v", err)
	}

	if len(publisher.published) != 1 || publisher.published[0].OrderingKey != "event-b" {
		t.Errorf("published %v, want only event-b while event-a is held behind its failed entry", publisher.published)
	}
}

func TestDispatchPendingAbandonsAnEntryAtTheAttemptLimit(t *testing.T) {
	t.Parallel()

	outbox := NewMockOutboxRepository(gomock.NewController(t))
	dispatcher := application.NewOutboxDispatcher(outbox, &fakePublisher{fails: func(domain.OutboxMessage) bool { return true }}, 3)

	entry := pendingEntry(1, "event-a", 2)
	outbox.EXPECT().ListPending(gomock.Any(), gomock.Any()).Return([]domain.OutboxEntry{entry}, nil)
	outbox.EXPECT().Save(gomock.Any(), withState(entry, domain.OutboxAbandoned, 3)).Return(nil)

	if err := dispatcher.DispatchPending(context.Background()); err != nil {
		t.Fatalf("DispatchPending: %v", err)
	}
}

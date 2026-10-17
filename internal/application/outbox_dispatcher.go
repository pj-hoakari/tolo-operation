package application

//go:generate go tool mockgen -source=../repository/outbox.go -destination=mock_outbox_repository_test.go -package=application_test

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
)

const outboxBatchSize = 100

type Publisher interface {
	Publish(ctx context.Context, message domain.OutboxMessage) error
}

type OutboxDispatcher struct {
	outbox      repository.OutboxRepository
	publisher   Publisher
	maxAttempts int
}

func NewOutboxDispatcher(outbox repository.OutboxRepository, publisher Publisher, maxAttempts int) *OutboxDispatcher {
	return &OutboxDispatcher{outbox: outbox, publisher: publisher, maxAttempts: maxAttempts}
}

type orderingScope struct {
	topic domain.Topic
	key   string
}

func (d *OutboxDispatcher) DispatchPending(ctx context.Context) error {
	entries, err := d.outbox.ListPending(ctx, outboxBatchSize)
	if err != nil {
		return fmt.Errorf("list pending outbox messages: %w", err)
	}

	held := map[orderingScope]bool{}

	for _, entry := range entries {
		scope := orderingScope{topic: entry.Message.Topic, key: entry.Message.OrderingKey}
		if held[scope] {
			continue
		}

		if err := d.publisher.Publish(ctx, entry.Message); err != nil {
			held[scope] = true
			entry = entry.PublishFailed(d.maxAttempts)
			reportPublishFailure(ctx, entry, err)
		} else {
			entry = entry.Published()
		}

		if err := d.outbox.Save(ctx, entry); err != nil {
			return fmt.Errorf("save outbox message %d: %w", entry.ID, err)
		}
	}

	return nil
}

func (d *OutboxDispatcher) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := d.DispatchPending(ctx); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "dispatch outbox failed", "error", err)
			}
		}
	}
}

func reportPublishFailure(ctx context.Context, entry domain.OutboxEntry, err error) {
	attrs := []any{
		"outbox_id", entry.ID,
		"topic", string(entry.Message.Topic),
		"ordering_key", entry.Message.OrderingKey,
		"attempts", entry.Attempts,
		"error", err,
	}

	if entry.Status == domain.OutboxAbandoned {
		slog.ErrorContext(ctx, "outbox message abandoned after repeated publish failures", attrs...)

		return
	}

	slog.WarnContext(ctx, "publish failed; retrying on the next pass", attrs...)
}

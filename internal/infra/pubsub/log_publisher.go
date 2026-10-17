package pubsub

import (
	"context"
	"log/slog"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

type LogPublisher struct{}

func NewLogPublisher() *LogPublisher {
	return &LogPublisher{}
}

func (p *LogPublisher) Publish(ctx context.Context, message domain.OutboxMessage) error {
	slog.InfoContext(ctx, "pubsub publish (log-only skeleton)",
		"topic", string(message.Topic),
		"ordering_key", message.OrderingKey,
		"payload", message.Payload,
	)

	return nil
}

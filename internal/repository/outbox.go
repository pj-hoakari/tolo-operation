package repository

import (
	"context"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

type OutboxRepository interface {
	Enqueue(ctx context.Context, message domain.OutboxMessage) error
	ListPending(ctx context.Context, limit int) ([]domain.OutboxEntry, error)
	Save(ctx context.Context, entry domain.OutboxEntry) error
}

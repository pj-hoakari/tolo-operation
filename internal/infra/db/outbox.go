package db

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

type PostgresOutboxRepository struct {
	db *sqlx.DB
}

func NewPostgresOutboxRepository(db *sqlx.DB) *PostgresOutboxRepository {
	return &PostgresOutboxRepository{db: db}
}

type outboxRow struct {
	ID          int64  `db:"id"`
	Topic       string `db:"topic"`
	OrderingKey string `db:"ordering_key"`
	Payload     []byte `db:"payload"`
	Status      string `db:"status"`
	Attempts    int    `db:"attempts"`
}

func (r *PostgresOutboxRepository) Enqueue(ctx context.Context, message domain.OutboxMessage) error {
	_, err := Executor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO outbox_messages (topic, ordering_key, payload) VALUES ($1, $2, $3)`,
		string(message.Topic), message.OrderingKey, []byte(message.Payload))
	if err != nil {
		return fmt.Errorf("insert outbox message: %w", err)
	}

	return nil
}

func (r *PostgresOutboxRepository) ListPending(ctx context.Context, limit int) ([]domain.OutboxEntry, error) {
	var rows []outboxRow

	err := sqlx.SelectContext(ctx, Executor(ctx, r.db), &rows, `
		SELECT id, topic, ordering_key, payload, status, attempts
		FROM outbox_messages
		WHERE status = 'pending'
		ORDER BY id
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("select pending outbox messages: %w", err)
	}

	entries := make([]domain.OutboxEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, domain.OutboxEntry{
			ID: row.ID,
			Message: domain.OutboxMessage{
				Topic:       domain.Topic(row.Topic),
				OrderingKey: row.OrderingKey,
				Payload:     row.Payload,
			},
			Status:   domain.OutboxStatus(row.Status),
			Attempts: row.Attempts,
		})
	}

	return entries, nil
}

func (r *PostgresOutboxRepository) Save(ctx context.Context, entry domain.OutboxEntry) error {
	_, err := Executor(ctx, r.db).ExecContext(ctx, `
		UPDATE outbox_messages SET status = $2, attempts = $3 WHERE id = $1`,
		entry.ID, string(entry.Status), entry.Attempts)
	if err != nil {
		return fmt.Errorf("update outbox message: %w", err)
	}

	return nil
}

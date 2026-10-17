package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

type PostgresConnectionStateRepository struct {
	db *sqlx.DB
}

func NewPostgresConnectionStateRepository(db *sqlx.DB) *PostgresConnectionStateRepository {
	return &PostgresConnectionStateRepository{db: db}
}

func (r *PostgresConnectionStateRepository) Save(ctx context.Context, state domain.ConnectionState) error {
	_, err := Executor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO connection_states (tenant_public_id, event_id, user_id, device_id, connected, at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_public_id, event_id, user_id, device_id) DO UPDATE
		SET connected = EXCLUDED.connected, at = EXCLUDED.at
		WHERE connection_states.at < EXCLUDED.at`,
		state.TenantPublicID, state.EventID, state.UserID, state.DeviceID, state.Connected, state.At)
	if err != nil {
		return fmt.Errorf("upsert connection state: %w", err)
	}

	return nil
}

func (r *PostgresConnectionStateRepository) ListByEvent(
	ctx context.Context,
	tenantPublicID, eventID string,
) ([]domain.ConnectionState, error) {
	var rows []struct {
		TenantPublicID string    `db:"tenant_public_id"`
		EventID        string    `db:"event_id"`
		UserID         string    `db:"user_id"`
		DeviceID       string    `db:"device_id"`
		Connected      bool      `db:"connected"`
		At             time.Time `db:"at"`
	}

	err := sqlx.SelectContext(ctx, Executor(ctx, r.db), &rows, `
		SELECT tenant_public_id, event_id, user_id, device_id, connected, at
		FROM connection_states
		WHERE tenant_public_id = $1 AND event_id = $2
		ORDER BY user_id, device_id`, tenantPublicID, eventID)
	if err != nil {
		return nil, fmt.Errorf("select connection states: %w", err)
	}

	states := make([]domain.ConnectionState, 0, len(rows))
	for _, row := range rows {
		states = append(states, domain.ConnectionState(row))
	}

	return states, nil
}

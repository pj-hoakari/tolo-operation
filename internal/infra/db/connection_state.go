package db

import (
	"context"
	"fmt"

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

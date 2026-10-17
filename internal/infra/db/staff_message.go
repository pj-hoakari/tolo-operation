package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
)

type PostgresStaffMessageRepository struct {
	db *sqlx.DB
}

func NewPostgresStaffMessageRepository(db *sqlx.DB) *PostgresStaffMessageRepository {
	return &PostgresStaffMessageRepository{db: db}
}

type staffMessageRow struct {
	ID              string    `db:"message_id"`
	TenantPublicID  string    `db:"tenant_public_id"`
	EventID         string    `db:"event_id"`
	SenderUserID    string    `db:"sender_user_id"`
	Kind            string    `db:"kind"`
	Audience        string    `db:"audience"`
	Body            string    `db:"body"`
	RelatedRouteIDs []byte    `db:"related_route_ids"`
	SentAt          time.Time `db:"sent_at"`
}

func (r *PostgresStaffMessageRepository) Create(ctx context.Context, message domain.StaffMessage) error {
	routeIDs, err := json.Marshal(message.RelatedRouteIDs)
	if err != nil {
		return fmt.Errorf("encode related route IDs: %w", err)
	}

	_, err = Executor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO staff_messages
			(message_id, tenant_public_id, event_id, sender_user_id, kind, audience, body, related_route_ids, sent_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		string(message.ID), message.TenantPublicID, message.EventID, message.SenderUserID,
		string(message.Kind), string(message.Audience), message.Body, routeIDs, message.SentAt)
	if err != nil {
		return fmt.Errorf("insert staff message: %w", err)
	}

	return nil
}

func (r *PostgresStaffMessageRepository) Revoke(
	ctx context.Context,
	tenantPublicID, eventID string,
	id domain.MessageID,
	revokedAt time.Time,
) error {
	result, err := Executor(ctx, r.db).ExecContext(ctx, `
		UPDATE staff_messages
		SET revoked_at = COALESCE(revoked_at, $4)
		WHERE tenant_public_id = $1 AND event_id = $2 AND message_id = $3`,
		tenantPublicID, eventID, string(id), revokedAt)
	if err != nil {
		return fmt.Errorf("revoke staff message: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("revoke staff message: %w", err)
	}

	if affected == 0 {
		return repository.ErrStaffMessageNotFound
	}

	return nil
}

func (r *PostgresStaffMessageRepository) ListActive(
	ctx context.Context,
	tenantPublicID, eventID string,
	audiences []domain.Audience,
) ([]domain.StaffMessage, error) {
	audienceValues := make([]string, 0, len(audiences))
	for _, audience := range audiences {
		audienceValues = append(audienceValues, string(audience))
	}

	var rows []staffMessageRow

	err := sqlx.SelectContext(ctx, Executor(ctx, r.db), &rows, `
		SELECT message_id, tenant_public_id, event_id, sender_user_id, kind, audience, body, related_route_ids, sent_at
		FROM staff_messages
		WHERE tenant_public_id = $1 AND event_id = $2 AND revoked_at IS NULL AND audience = ANY($3)
		ORDER BY sent_at, id`, tenantPublicID, eventID, audienceValues)
	if err != nil {
		return nil, fmt.Errorf("select staff messages: %w", err)
	}

	messages := make([]domain.StaffMessage, 0, len(rows))

	for _, row := range rows {
		message := domain.StaffMessage{
			ID:              domain.MessageID(row.ID),
			TenantPublicID:  row.TenantPublicID,
			EventID:         row.EventID,
			SenderUserID:    row.SenderUserID,
			Kind:            domain.MessageKind(row.Kind),
			Audience:        domain.Audience(row.Audience),
			Body:            row.Body,
			RelatedRouteIDs: nil,
			SentAt:          row.SentAt,
		}

		if err := json.Unmarshal(row.RelatedRouteIDs, &message.RelatedRouteIDs); err != nil {
			return nil, fmt.Errorf("decode related route IDs: %w", err)
		}

		messages = append(messages, message)
	}

	return messages, nil
}

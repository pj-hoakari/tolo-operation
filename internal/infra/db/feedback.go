package db

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

type PostgresFeedbackRepository struct {
	db *sqlx.DB
}

func NewPostgresFeedbackRepository(db *sqlx.DB) *PostgresFeedbackRepository {
	return &PostgresFeedbackRepository{db: db}
}

func (r *PostgresFeedbackRepository) Record(ctx context.Context, values domain.FeedbackValues) error {
	tags := make([]string, 0, len(values.Entries))
	kinds := make([]string, 0, len(values.Entries))
	numbers := make([]float64, 0, len(values.Entries))

	for _, entry := range values.Entries {
		tags = append(tags, entry.AttributeTag)
		kinds = append(kinds, entry.Kind)
		numbers = append(numbers, entry.Value)
	}

	_, err := Executor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO feedback_values (tenant_public_id, event_id, computed_at, attribute_tag, kind, value)
		SELECT $1, $2, $3, entry.attribute_tag, entry.kind, entry.value
		FROM unnest($4::text[], $5::text[], $6::double precision[]) AS entry (attribute_tag, kind, value)
		ON CONFLICT (tenant_public_id, event_id, computed_at, attribute_tag, kind) DO NOTHING`,
		values.TenantPublicID, values.EventID, values.ComputedAt, tags, kinds, numbers)
	if err != nil {
		return fmt.Errorf("insert feedback values: %w", err)
	}

	return nil
}

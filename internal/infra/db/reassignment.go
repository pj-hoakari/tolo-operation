package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
)

type PostgresReassignmentRepository struct {
	db *sqlx.DB
}

func NewPostgresReassignmentRepository(db *sqlx.DB) *PostgresReassignmentRepository {
	return &PostgresReassignmentRepository{db: db}
}

type reassignmentRow struct {
	ID             string    `db:"reassignment_id"`
	TenantPublicID string    `db:"tenant_public_id"`
	EventID        string    `db:"event_id"`
	StaffUserID    string    `db:"staff_user_id"`
	TargetKind     string    `db:"target_kind"`
	TargetID       string    `db:"target_id"`
	RoutePosition  *float64  `db:"route_position"`
	Status         string    `db:"status"`
	DirectedAt     time.Time `db:"directed_at"`
}

func (r reassignmentRow) toDomain() (domain.Reassignment, error) {
	target, err := domain.NewReassignmentTarget(domain.TargetKind(r.TargetKind), r.TargetID, r.RoutePosition)
	if err != nil {
		return domain.Reassignment{}, fmt.Errorf("restore reassignment target: %w", err)
	}

	return domain.Reassignment{
		ID:             domain.ReassignmentID(r.ID),
		TenantPublicID: r.TenantPublicID,
		EventID:        r.EventID,
		StaffUserID:    r.StaffUserID,
		Target:         target,
		Status:         domain.ReassignmentStatus(r.Status),
		DirectedAt:     r.DirectedAt,
	}, nil
}

const selectReassignmentColumns = `
	reassignment_id, tenant_public_id, event_id, staff_user_id,
	target_kind, target_id, route_position, status, directed_at`

func (r *PostgresReassignmentRepository) Create(ctx context.Context, reassignment domain.Reassignment) error {
	var routePosition *float64
	if position, ok := reassignment.Target.RoutePosition(); ok {
		routePosition = &position
	}

	_, err := Executor(ctx, r.db).ExecContext(ctx, `
		INSERT INTO reassignments
			(reassignment_id, tenant_public_id, event_id, staff_user_id, target_kind, target_id, route_position, status, directed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		string(reassignment.ID), reassignment.TenantPublicID, reassignment.EventID, reassignment.StaffUserID,
		string(reassignment.Target.Kind()), reassignment.Target.ID(), routePosition,
		string(reassignment.Status), reassignment.DirectedAt)
	if err != nil {
		return fmt.Errorf("insert reassignment: %w", err)
	}

	return nil
}

func (r *PostgresReassignmentRepository) FindByID(
	ctx context.Context,
	tenantPublicID string,
	id domain.ReassignmentID,
) (domain.Reassignment, error) {
	var row reassignmentRow

	err := sqlx.GetContext(ctx, Executor(ctx, r.db), &row, `
		SELECT`+selectReassignmentColumns+`
		FROM reassignments
		WHERE tenant_public_id = $1 AND reassignment_id = $2`, tenantPublicID, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Reassignment{}, repository.ErrReassignmentNotFound
	}

	if err != nil {
		return domain.Reassignment{}, fmt.Errorf("select reassignment: %w", err)
	}

	return row.toDomain()
}

func (r *PostgresReassignmentRepository) SaveStatus(ctx context.Context, reassignment domain.Reassignment) error {
	result, err := Executor(ctx, r.db).ExecContext(ctx, `
		UPDATE reassignments
		SET status = $3
		WHERE tenant_public_id = $1 AND reassignment_id = $2`,
		reassignment.TenantPublicID, string(reassignment.ID), string(reassignment.Status))
	if err != nil {
		return fmt.Errorf("update reassignment status: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update reassignment status: %w", err)
	}

	if affected == 0 {
		return repository.ErrReassignmentNotFound
	}

	return nil
}

func (r *PostgresReassignmentRepository) ListCurrent(
	ctx context.Context,
	tenantPublicID, eventID string,
) ([]domain.Reassignment, error) {
	var rows []reassignmentRow

	err := sqlx.SelectContext(ctx, Executor(ctx, r.db), &rows, `
		SELECT DISTINCT ON (staff_user_id)`+selectReassignmentColumns+`
		FROM reassignments
		WHERE tenant_public_id = $1 AND event_id = $2
		ORDER BY staff_user_id, directed_at DESC, id DESC`, tenantPublicID, eventID)
	if err != nil {
		return nil, fmt.Errorf("select current reassignments: %w", err)
	}

	reassignments := make([]domain.Reassignment, 0, len(rows))

	for _, row := range rows {
		reassignment, err := row.toDomain()
		if err != nil {
			return nil, err
		}

		reassignments = append(reassignments, reassignment)
	}

	return reassignments, nil
}

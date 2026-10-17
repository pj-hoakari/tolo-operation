package application

//go:generate go tool mockgen -source=../repository/reassignment.go -destination=mock_reassignment_repository_test.go -package=application_test

import (
	"context"
	"fmt"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
	"github.com/pj-hoakari/tolo-operation/internal/tenantctx"
)

type DirectReassignmentInput struct {
	EventID       string
	StaffUserID   string
	TargetKind    domain.TargetKind
	TargetID      string
	RoutePosition *float64
}

type ApplyReassignmentInput struct {
	ReassignmentID string
}

type ListReassignmentsInput struct {
	EventID string
}

type OperationControlUseCases interface {
	DirectReassignment(ctx context.Context, input DirectReassignmentInput) (domain.Reassignment, error)
	ApplyReassignment(ctx context.Context, input ApplyReassignmentInput) (domain.Reassignment, error)
	ListReassignments(ctx context.Context, input ListReassignmentsInput) ([]domain.Reassignment, error)
}

type OperationControlService struct {
	reassignments repository.ReassignmentRepository
	now           func() time.Time
}

func NewOperationControlService(reassignments repository.ReassignmentRepository, now func() time.Time) *OperationControlService {
	return &OperationControlService{reassignments: reassignments, now: now}
}

func (s *OperationControlService) DirectReassignment(ctx context.Context, input DirectReassignmentInput) (domain.Reassignment, error) {
	tenantPublicID, err := eventTenant(ctx, input.EventID)
	if err != nil {
		return domain.Reassignment{}, err
	}

	target, err := domain.NewReassignmentTarget(input.TargetKind, input.TargetID, input.RoutePosition)
	if err != nil {
		return domain.Reassignment{}, err
	}

	reassignment, err := domain.DirectReassignment(tenantPublicID, input.EventID, input.StaffUserID, target, s.now())
	if err != nil {
		return domain.Reassignment{}, err
	}

	if err := s.reassignments.Create(ctx, reassignment); err != nil {
		return domain.Reassignment{}, fmt.Errorf("create reassignment: %w", err)
	}

	return reassignment, nil
}

func (s *OperationControlService) ApplyReassignment(ctx context.Context, input ApplyReassignmentInput) (domain.Reassignment, error) {
	tenantPublicID, ok := tenantctx.TenantPublicIDFromContext(ctx)
	if !ok {
		return domain.Reassignment{}, tenantctx.ErrMissing
	}

	id, err := domain.ParseReassignmentID(input.ReassignmentID)
	if err != nil {
		return domain.Reassignment{}, err
	}

	reassignment, err := s.reassignments.FindByID(ctx, tenantPublicID, id)
	if err != nil {
		return domain.Reassignment{}, fmt.Errorf("find reassignment: %w", err)
	}

	if err := tenantctx.EnsureEvent(ctx, reassignment.EventID); err != nil {
		return domain.Reassignment{}, err
	}

	applied, changed := reassignment.Apply()
	if !changed {
		return applied, nil
	}

	if err := s.reassignments.SaveStatus(ctx, applied); err != nil {
		return domain.Reassignment{}, fmt.Errorf("save reassignment: %w", err)
	}

	return applied, nil
}

func (s *OperationControlService) ListReassignments(ctx context.Context, input ListReassignmentsInput) ([]domain.Reassignment, error) {
	tenantPublicID, err := eventTenant(ctx, input.EventID)
	if err != nil {
		return nil, err
	}

	reassignments, err := s.reassignments.ListCurrent(ctx, tenantPublicID, input.EventID)
	if err != nil {
		return nil, fmt.Errorf("list reassignments: %w", err)
	}

	return reassignments, nil
}

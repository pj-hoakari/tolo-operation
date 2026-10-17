package application

//go:generate go tool mockgen -source=../repository/connection_state.go -destination=mock_connection_state_repository_test.go -package=application_test
//go:generate go tool mockgen -source=../repository/feedback.go -destination=mock_feedback_repository_test.go -package=application_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
	"github.com/pj-hoakari/tolo-operation/internal/tenantctx"
)

var ErrForeignUser = errors.New("user ID does not match the token subject")

type UpdateConnectionStateInput struct {
	UserID    string
	DeviceID  string
	Connected bool
	At        time.Time
}

type RecordFeedbackValuesInput struct {
	TenantID   string
	EventID    string
	ComputedAt time.Time
	Entries    []domain.FeedbackEntry
}

type DeliveryCoordinationUseCases interface {
	UpdateConnectionState(ctx context.Context, input UpdateConnectionStateInput) error
	RecordFeedbackValues(ctx context.Context, input RecordFeedbackValuesInput) error
}

type DeliveryCoordinationService struct {
	states   repository.ConnectionStateRepository
	feedback repository.FeedbackRepository
}

func NewDeliveryCoordinationService(
	states repository.ConnectionStateRepository,
	feedback repository.FeedbackRepository,
) *DeliveryCoordinationService {
	return &DeliveryCoordinationService{states: states, feedback: feedback}
}

func (s *DeliveryCoordinationService) UpdateConnectionState(ctx context.Context, input UpdateConnectionStateInput) error {
	tenantPublicID, ok := tenantctx.TenantPublicIDFromContext(ctx)
	if !ok {
		return tenantctx.ErrMissing
	}

	eventID, ok := tenantctx.EventPublicIDFromContext(ctx)
	if !ok {
		return tenantctx.ErrEventMissing
	}

	subject, ok := tenantctx.SubjectFromContext(ctx)
	if !ok {
		return tenantctx.ErrSubjectMissing
	}

	state, err := domain.NewConnectionState(tenantPublicID, eventID, input.UserID, input.DeviceID, input.Connected, input.At)
	if err != nil {
		return err
	}

	if state.UserID != subject {
		return ErrForeignUser
	}

	if err := s.states.Save(ctx, state); err != nil {
		return fmt.Errorf("save connection state: %w", err)
	}

	return nil
}

func (s *DeliveryCoordinationService) RecordFeedbackValues(ctx context.Context, input RecordFeedbackValuesInput) error {
	tenantPublicID, eventID, err := requestScope(ctx, input.TenantID, input.EventID)
	if err != nil {
		return err
	}

	values, err := domain.NewFeedbackValues(tenantPublicID, eventID, input.ComputedAt, input.Entries)
	if err != nil {
		return err
	}

	if err := s.feedback.Record(ctx, values); err != nil {
		return fmt.Errorf("record feedback values: %w", err)
	}

	return nil
}

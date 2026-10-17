package application

//go:generate go tool mockgen -source=../repository/connection_state.go -destination=mock_connection_state_repository_test.go -package=application_test
//go:generate go tool mockgen -source=../repository/feedback.go -destination=mock_feedback_repository_test.go -package=application_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
)

var ErrForeignUser = errors.New("user ID does not match the token subject")

type UpdateConnectionStateInput struct {
	TenantID  string
	EventID   string
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

type RequestProposalDeliveryInput struct {
	TenantID        string
	EventID         string
	ProposalKind    domain.ProposalKind
	ProposalContent json.RawMessage
}

type DeliveryResult struct {
	DeliveryID        domain.DeliveryID
	RealtimeCount     int
	NotificationCount int
}

type PushRequest struct {
	EventID          string
	RecipientUserIDs []string
	Message          domain.PushMessage
}

type PushResult struct {
	UserID        string
	Sent          bool
	FailureReason string
}

type Notifier interface {
	SendPush(ctx context.Context, request PushRequest) ([]PushResult, error)
}

type DeliveryCoordinationUseCases interface {
	RequestProposalDelivery(ctx context.Context, input RequestProposalDeliveryInput) (DeliveryResult, error)
	UpdateConnectionState(ctx context.Context, input UpdateConnectionStateInput) error
	RecordFeedbackValues(ctx context.Context, input RecordFeedbackValuesInput) error
}

type DeliveryCoordinationService struct {
	states   repository.ConnectionStateRepository
	feedback repository.FeedbackRepository
	outbox   repository.OutboxRepository
	notifier Notifier
}

func NewDeliveryCoordinationService(
	states repository.ConnectionStateRepository,
	feedback repository.FeedbackRepository,
	outbox repository.OutboxRepository,
	notifier Notifier,
) *DeliveryCoordinationService {
	return &DeliveryCoordinationService{states: states, feedback: feedback, outbox: outbox, notifier: notifier}
}

func (s *DeliveryCoordinationService) RequestProposalDelivery(
	ctx context.Context,
	input RequestProposalDeliveryInput,
) (DeliveryResult, error) {
	tenantPublicID, eventID, err := requestScope(ctx, input.TenantID, input.EventID)
	if err != nil {
		return DeliveryResult{}, err
	}

	proposal, err := domain.NewProposal(input.ProposalKind, input.ProposalContent)
	if err != nil {
		return DeliveryResult{}, err
	}

	deliveryID, err := domain.NewDeliveryID()
	if err != nil {
		return DeliveryResult{}, err
	}

	states, err := s.states.ListByEvent(ctx, tenantPublicID, eventID)
	if err != nil {
		return DeliveryResult{}, fmt.Errorf("list connection states: %w", err)
	}

	route := domain.RouteDelivery(domain.SelectRecipients(states), states)

	if len(route.Realtime) > 0 {
		message, err := domain.NewOutboxMessage(domain.DeliveryOrder{
			DeliveryID:       deliveryID,
			EventID:          eventID,
			RecipientUserIDs: route.Realtime,
			Payload:          proposal,
		})
		if err != nil {
			return DeliveryResult{}, err
		}

		if err := s.outbox.Enqueue(ctx, message); err != nil {
			return DeliveryResult{}, fmt.Errorf("enqueue %s: %w", message.Topic, err)
		}
	}

	return DeliveryResult{
		DeliveryID:        deliveryID,
		RealtimeCount:     len(route.Realtime),
		NotificationCount: s.push(ctx, eventID, deliveryID, proposal, route.Push),
	}, nil
}

func (s *DeliveryCoordinationService) push(
	ctx context.Context,
	eventID string,
	deliveryID domain.DeliveryID,
	proposal domain.Proposal,
	userIDs []string,
) int {
	if len(userIDs) == 0 {
		return 0
	}

	results, err := s.notifier.SendPush(ctx, PushRequest{
		EventID:          eventID,
		RecipientUserIDs: userIDs,
		Message:          domain.ProposalArrival(eventID, deliveryID, proposal),
	})
	if err != nil {
		slog.WarnContext(ctx, "send push failed", "delivery_id", string(deliveryID), "recipients", len(userIDs), "error", err)

		return 0
	}

	sent := map[string]bool{}

	for _, result := range results {
		if result.Sent {
			sent[result.UserID] = true

			continue
		}

		slog.WarnContext(ctx, "push not sent",
			"delivery_id", string(deliveryID), "user_id", result.UserID, "reason", result.FailureReason)
	}

	return len(sent)
}

func (s *DeliveryCoordinationService) UpdateConnectionState(ctx context.Context, input UpdateConnectionStateInput) error {
	tenantPublicID, eventID, err := requestScope(ctx, input.TenantID, input.EventID)
	if err != nil {
		return err
	}

	subjectID, err := subject(ctx)
	if err != nil {
		return err
	}

	state, err := domain.NewConnectionState(tenantPublicID, eventID, input.UserID, input.DeviceID, input.Connected, input.At)
	if err != nil {
		return err
	}

	if state.UserID != subjectID {
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

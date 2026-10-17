package application

//go:generate go tool mockgen -source=../repository/staff_message.go -destination=mock_staff_message_repository_test.go -package=application_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
	"github.com/pj-hoakari/tolo-operation/internal/tenantctx"
)

var ErrGuestAudienceOnly = errors.New("service callers may list guest messages only")

type SendStaffMessageInput struct {
	EventID  string
	Audience domain.Audience
	Body     string
}

type RevokeStaffMessageInput struct {
	EventID   string
	MessageID string
}

type ShareGuidanceInfoInput struct {
	EventID         string
	Body            string
	RelatedRouteIDs []string
}

type ListStaffMessagesInput struct {
	EventID  string
	Audience domain.Audience
}

type StaffCommunicationUseCases interface {
	SendStaffMessage(ctx context.Context, input SendStaffMessageInput) (domain.StaffMessage, error)
	RevokeStaffMessage(ctx context.Context, input RevokeStaffMessageInput) error
	ShareGuidanceInfo(ctx context.Context, input ShareGuidanceInfoInput) (domain.StaffMessage, error)
	ListStaffMessages(ctx context.Context, input ListStaffMessagesInput) ([]domain.StaffMessage, error)
}

type StaffCommunicationService struct {
	messages repository.StaffMessageRepository
	now      func() time.Time
}

func NewStaffCommunicationService(messages repository.StaffMessageRepository, now func() time.Time) *StaffCommunicationService {
	return &StaffCommunicationService{messages: messages, now: now}
}

func (s *StaffCommunicationService) SendStaffMessage(ctx context.Context, input SendStaffMessageInput) (domain.StaffMessage, error) {
	tenantPublicID, senderUserID, err := sender(ctx, input.EventID)
	if err != nil {
		return domain.StaffMessage{}, err
	}

	message, err := domain.NewStaffMessage(tenantPublicID, input.EventID, senderUserID, input.Audience, input.Body, s.now())
	if err != nil {
		return domain.StaffMessage{}, err
	}

	if err := s.messages.Create(ctx, message); err != nil {
		return domain.StaffMessage{}, fmt.Errorf("create staff message: %w", err)
	}

	return message, nil
}

func (s *StaffCommunicationService) ShareGuidanceInfo(ctx context.Context, input ShareGuidanceInfoInput) (domain.StaffMessage, error) {
	tenantPublicID, senderUserID, err := sender(ctx, input.EventID)
	if err != nil {
		return domain.StaffMessage{}, err
	}

	message, err := domain.NewGuidanceInfo(tenantPublicID, input.EventID, senderUserID, input.Body, input.RelatedRouteIDs, s.now())
	if err != nil {
		return domain.StaffMessage{}, err
	}

	if err := s.messages.Create(ctx, message); err != nil {
		return domain.StaffMessage{}, fmt.Errorf("create staff message: %w", err)
	}

	return message, nil
}

func (s *StaffCommunicationService) RevokeStaffMessage(ctx context.Context, input RevokeStaffMessageInput) error {
	tenantPublicID, err := eventTenant(ctx, input.EventID)
	if err != nil {
		return err
	}

	id, err := domain.ParseMessageID(input.MessageID)
	if err != nil {
		return err
	}

	if err := s.messages.Revoke(ctx, tenantPublicID, input.EventID, id, s.now()); err != nil {
		return fmt.Errorf("revoke staff message: %w", err)
	}

	return nil
}

func (s *StaffCommunicationService) ListStaffMessages(ctx context.Context, input ListStaffMessagesInput) ([]domain.StaffMessage, error) {
	if err := tenantctx.EnsureEvent(ctx, input.EventID); err != nil {
		return nil, err
	}

	if tenantctx.IsServiceCall(ctx) && input.Audience != domain.AudienceGuest {
		return nil, ErrGuestAudienceOnly
	}

	messages, err := s.messages.ListActive(ctx, input.EventID, domain.AudiencesVisibleTo(input.Audience))
	if err != nil {
		return nil, fmt.Errorf("list staff messages: %w", err)
	}

	return messages, nil
}

func eventTenant(ctx context.Context, eventID string) (string, error) {
	if err := tenantctx.EnsureEvent(ctx, eventID); err != nil {
		return "", err
	}

	tenantPublicID, ok := tenantctx.TenantPublicIDFromContext(ctx)
	if !ok {
		return "", tenantctx.ErrMissing
	}

	return tenantPublicID, nil
}

func sender(ctx context.Context, eventID string) (string, string, error) {
	tenantPublicID, err := eventTenant(ctx, eventID)
	if err != nil {
		return "", "", err
	}

	subject, ok := tenantctx.SubjectFromContext(ctx)
	if !ok {
		return "", "", tenantctx.ErrSubjectMissing
	}

	return tenantPublicID, subject, nil
}

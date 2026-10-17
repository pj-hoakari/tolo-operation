package application

//go:generate go tool mockgen -source=../repository/staff_message.go -destination=mock_staff_message_repository_test.go -package=application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
	"github.com/pj-hoakari/tolo-operation/internal/tenantctx"
)

var ErrGuestAudienceOnly = errors.New("service callers may list guest messages only")

type SendStaffMessageInput struct {
	TenantID string
	EventID  string
	Audience domain.Audience
	Body     string
}

type RevokeStaffMessageInput struct {
	TenantID  string
	EventID   string
	MessageID string
}

type ShareGuidanceInfoInput struct {
	TenantID        string
	EventID         string
	Body            string
	RelatedRouteIDs []string
}

type ListStaffMessagesInput struct {
	TenantID string
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
	tenantPublicID, eventID, err := requestScope(ctx, input.TenantID, input.EventID)
	if err != nil {
		return domain.StaffMessage{}, err
	}

	senderUserID, err := subject(ctx)
	if err != nil {
		return domain.StaffMessage{}, err
	}

	message, err := domain.NewStaffMessage(tenantPublicID, eventID, senderUserID, input.Audience, input.Body, s.now())
	if err != nil {
		return domain.StaffMessage{}, err
	}

	if err := s.messages.Create(ctx, message); err != nil {
		return domain.StaffMessage{}, fmt.Errorf("create staff message: %w", err)
	}

	return message, nil
}

func (s *StaffCommunicationService) ShareGuidanceInfo(ctx context.Context, input ShareGuidanceInfoInput) (domain.StaffMessage, error) {
	tenantPublicID, eventID, err := requestScope(ctx, input.TenantID, input.EventID)
	if err != nil {
		return domain.StaffMessage{}, err
	}

	senderUserID, err := subject(ctx)
	if err != nil {
		return domain.StaffMessage{}, err
	}

	message, err := domain.NewGuidanceInfo(tenantPublicID, eventID, senderUserID, input.Body, input.RelatedRouteIDs, s.now())
	if err != nil {
		return domain.StaffMessage{}, err
	}

	if err := s.messages.Create(ctx, message); err != nil {
		return domain.StaffMessage{}, fmt.Errorf("create staff message: %w", err)
	}

	return message, nil
}

func (s *StaffCommunicationService) RevokeStaffMessage(ctx context.Context, input RevokeStaffMessageInput) error {
	tenantPublicID, eventID, err := requestScope(ctx, input.TenantID, input.EventID)
	if err != nil {
		return err
	}

	id, err := domain.ParseMessageID(input.MessageID)
	if err != nil {
		return err
	}

	if err := s.messages.Revoke(ctx, tenantPublicID, eventID, id, s.now()); err != nil {
		return fmt.Errorf("revoke staff message: %w", err)
	}

	return nil
}

func (s *StaffCommunicationService) ListStaffMessages(ctx context.Context, input ListStaffMessagesInput) ([]domain.StaffMessage, error) {
	tenantPublicID, eventID, err := requestScope(ctx, input.TenantID, input.EventID)
	if err != nil {
		return nil, err
	}

	if tenantctx.IsServiceCall(ctx) && input.Audience != domain.AudienceGuest {
		return nil, ErrGuestAudienceOnly
	}

	messages, err := s.messages.ListActive(ctx, tenantPublicID, eventID, domain.AudiencesVisibleTo(input.Audience))
	if err != nil {
		return nil, fmt.Errorf("list staff messages: %w", err)
	}

	return messages, nil
}

func requestScope(ctx context.Context, tenantID, eventID string) (string, string, error) {
	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return "", "", domain.ErrEventRequired
	}

	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return "", "", domain.ErrTenantRequired
	}

	if err := tenantctx.VerifyOwnership(ctx, tenantID); err != nil {
		return "", "", err
	}

	if err := tenantctx.EnsureEvent(ctx, eventID); err != nil {
		return "", "", err
	}

	return tenantID, eventID, nil
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

func subject(ctx context.Context) (string, error) {
	subject, ok := tenantctx.SubjectFromContext(ctx)
	if !ok {
		return "", tenantctx.ErrSubjectMissing
	}

	return subject, nil
}

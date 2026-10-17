package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"

	"github.com/pj-hoakari/tolo-operation/internal/application"
	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
	"github.com/pj-hoakari/tolo-operation/internal/tenantctx"
)

const (
	testTenantPublicID = "a1b2c3d4e5f60718"
	testEventPublicID  = "0f1e2d3c4b5a6978"
	otherEventPublicID = "9876543210fedcba"
	testSubject        = "staff-user-1"
	testMessageID      = "1122334455667788"
)

var testNow = time.Date(2026, time.October, 17, 9, 0, 0, 0, time.UTC)

func clock() time.Time { return testNow }

func withEventToken(eventPublicID string) context.Context {
	return internaljwt.ContextWithClaims(context.Background(), internaljwt.Claims{
		TokenUse:       internaljwt.TokenUseEventAccess,
		Subject:        testSubject,
		TenantPublicID: testTenantPublicID,
		EventPublicID:  eventPublicID,
	})
}

func withServiceToken() context.Context {
	return internaljwt.ContextWithClaims(context.Background(), internaljwt.Claims{
		TokenUse: internaljwt.TokenUseService,
		Subject:  "tolo-guest",
	})
}

func newStaffCommunication(t *testing.T) (*application.StaffCommunicationService, *MockStaffMessageRepository) {
	t.Helper()

	messages := NewMockStaffMessageRepository(gomock.NewController(t))

	return application.NewStaffCommunicationService(messages, clock), messages
}

func TestSendStaffMessage(t *testing.T) {
	t.Parallel()

	service, messages := newStaffCommunication(t)

	var stored domain.StaffMessage

	messages.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, message domain.StaffMessage) error {
		stored = message

		return nil
	})

	message, err := service.SendStaffMessage(withEventToken(testEventPublicID), application.SendStaffMessageInput{
		EventID:  testEventPublicID,
		Audience: domain.AudienceBoth,
		Body:     "gate B is open",
	})
	if err != nil {
		t.Fatalf("SendStaffMessage: %v", err)
	}

	if message.TenantPublicID != testTenantPublicID || message.EventID != testEventPublicID ||
		message.SenderUserID != testSubject || message.Kind != domain.MessageKindStaffMessage ||
		message.Audience != domain.AudienceBoth || message.Body != "gate B is open" || !message.SentAt.Equal(testNow) {
		t.Errorf("SendStaffMessage = %+v, want the message sent by the token subject in the token tenant", message)
	}

	if stored.ID != message.ID {
		t.Errorf("stored message %q, want the returned message %q", stored.ID, message.ID)
	}
}

func TestShareGuidanceInfoStoresAStaffOnlyGuidanceMessage(t *testing.T) {
	t.Parallel()

	service, messages := newStaffCommunication(t)
	messages.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

	message, err := service.ShareGuidanceInfo(withEventToken(testEventPublicID), application.ShareGuidanceInfoInput{
		EventID:         testEventPublicID,
		Body:            "one way from A to B",
		RelatedRouteIDs: []string{"route-a"},
	})
	if err != nil {
		t.Fatalf("ShareGuidanceInfo: %v", err)
	}

	if message.Kind != domain.MessageKindGuidanceInfo || message.Audience != domain.AudienceStaff ||
		!slices.Equal(message.RelatedRouteIDs, []string{"route-a"}) {
		t.Errorf("ShareGuidanceInfo = %+v, want staff guidance info for route-a", message)
	}
}

func TestStaffMessageCommandsRejectAnotherEvent(t *testing.T) {
	t.Parallel()

	service, _ := newStaffCommunication(t)
	ctx := withEventToken(testEventPublicID)

	_, sendErr := service.SendStaffMessage(ctx, application.SendStaffMessageInput{EventID: otherEventPublicID, Audience: domain.AudienceStaff, Body: "hi"})
	_, shareErr := service.ShareGuidanceInfo(ctx, application.ShareGuidanceInfoInput{EventID: otherEventPublicID, Body: "hi"})
	revokeErr := service.RevokeStaffMessage(ctx, application.RevokeStaffMessageInput{EventID: otherEventPublicID, MessageID: testMessageID})
	_, listErr := service.ListStaffMessages(ctx, application.ListStaffMessagesInput{EventID: otherEventPublicID})

	for name, err := range map[string]error{"send": sendErr, "share": shareErr, "revoke": revokeErr, "list": listErr} {
		if !errors.Is(err, tenantctx.ErrEventMismatch) {
			t.Errorf("%s error = %v, want %v", name, err, tenantctx.ErrEventMismatch)
		}
	}
}

func TestRevokeStaffMessage(t *testing.T) {
	t.Parallel()

	service, messages := newStaffCommunication(t)
	messages.EXPECT().Revoke(gomock.Any(), testTenantPublicID, testEventPublicID, domain.MessageID(testMessageID), testNow).Return(nil)

	err := service.RevokeStaffMessage(withEventToken(testEventPublicID), application.RevokeStaffMessageInput{
		EventID:   testEventPublicID,
		MessageID: testMessageID,
	})
	if err != nil {
		t.Fatalf("RevokeStaffMessage: %v", err)
	}
}

func TestRevokeStaffMessageReportsAnUnknownMessage(t *testing.T) {
	t.Parallel()

	service, messages := newStaffCommunication(t)
	messages.EXPECT().Revoke(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(repository.ErrStaffMessageNotFound)

	err := service.RevokeStaffMessage(withEventToken(testEventPublicID), application.RevokeStaffMessageInput{
		EventID:   testEventPublicID,
		MessageID: testMessageID,
	})
	if !errors.Is(err, repository.ErrStaffMessageNotFound) {
		t.Errorf("RevokeStaffMessage error = %v, want %v", err, repository.ErrStaffMessageNotFound)
	}
}

func TestListStaffMessagesFiltersByAudience(t *testing.T) {
	t.Parallel()

	service, messages := newStaffCommunication(t)
	listed := []domain.StaffMessage{{ID: testMessageID}}
	messages.EXPECT().
		ListActive(gomock.Any(), testEventPublicID, []domain.Audience{domain.AudienceStaff, domain.AudienceBoth}).
		Return(listed, nil)

	got, err := service.ListStaffMessages(withEventToken(testEventPublicID), application.ListStaffMessagesInput{
		EventID:  testEventPublicID,
		Audience: domain.AudienceStaff,
	})
	if err != nil {
		t.Fatalf("ListStaffMessages: %v", err)
	}

	if len(got) != 1 || got[0].ID != testMessageID {
		t.Errorf("ListStaffMessages = %+v, want %+v", got, listed)
	}
}

func TestListStaffMessagesLetsServiceCallersReadGuestMessagesOnly(t *testing.T) {
	t.Parallel()

	for _, audience := range []domain.Audience{"", domain.AudienceStaff, domain.AudienceBoth} {
		service, _ := newStaffCommunication(t)

		_, err := service.ListStaffMessages(withServiceToken(), application.ListStaffMessagesInput{
			EventID:  testEventPublicID,
			Audience: audience,
		})
		if !errors.Is(err, application.ErrGuestAudienceOnly) {
			t.Errorf("audience %q: ListStaffMessages error = %v, want %v", audience, err, application.ErrGuestAudienceOnly)
		}
	}

	service, messages := newStaffCommunication(t)
	messages.EXPECT().
		ListActive(gomock.Any(), testEventPublicID, []domain.Audience{domain.AudienceGuest, domain.AudienceBoth}).
		Return(nil, nil)

	if _, err := service.ListStaffMessages(withServiceToken(), application.ListStaffMessagesInput{
		EventID:  testEventPublicID,
		Audience: domain.AudienceGuest,
	}); err != nil {
		t.Errorf("ListStaffMessages(GUEST) as a service: %v", err)
	}
}

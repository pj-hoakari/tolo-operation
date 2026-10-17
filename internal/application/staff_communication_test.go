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
	testTenantPublicID  = "a1b2c3d4e5f60718"
	otherTenantPublicID = "8170f6e5d4c3b2a1"
	testEventPublicID   = "0f1e2d3c4b5a6978"
	otherEventPublicID  = "9876543210fedcba"
	testSubject         = "staff-user-1"
	testMessageID       = "1122334455667788"
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

type fakeTransactor struct {
	active bool
}

func (f *fakeTransactor) WithinTransaction(ctx context.Context, fn func(context.Context) error) error {
	f.active = true
	defer func() { f.active = false }()

	return fn(ctx)
}

type staffCommunicationDeps struct {
	messages     *MockStaffMessageRepository
	outbox       *MockOutboxRepository
	transactions *fakeTransactor
}

func newStaffCommunicationWithOutbox(t *testing.T) (*application.StaffCommunicationService, staffCommunicationDeps) {
	t.Helper()

	controller := gomock.NewController(t)
	deps := staffCommunicationDeps{
		messages:     NewMockStaffMessageRepository(controller),
		outbox:       NewMockOutboxRepository(controller),
		transactions: &fakeTransactor{},
	}

	return application.NewStaffCommunicationService(deps.messages, deps.outbox, deps.transactions, clock), deps
}

func newStaffCommunication(t *testing.T) (*application.StaffCommunicationService, *MockStaffMessageRepository) {
	t.Helper()

	service, deps := newStaffCommunicationWithOutbox(t)

	return service, deps.messages
}

func TestSendStaffMessage(t *testing.T) {
	t.Parallel()

	service, deps := newStaffCommunicationWithOutbox(t)
	messages := deps.messages
	deps.outbox.EXPECT().Enqueue(gomock.Any(), gomock.Any()).Return(nil)

	var stored domain.StaffMessage

	messages.EXPECT().Create(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, message domain.StaffMessage) error {
		stored = message

		return nil
	})

	message, err := service.SendStaffMessage(withEventToken(testEventPublicID), application.SendStaffMessageInput{
		TenantID: testTenantPublicID,
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
		TenantID:        testTenantPublicID,
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

func TestRevokeStaffMessage(t *testing.T) {
	t.Parallel()

	service, messages := newStaffCommunication(t)
	messages.EXPECT().Revoke(gomock.Any(), testTenantPublicID, testEventPublicID, domain.MessageID(testMessageID), testNow).Return(domain.AudienceStaff, true, nil)

	err := service.RevokeStaffMessage(withEventToken(testEventPublicID), application.RevokeStaffMessageInput{
		TenantID:  testTenantPublicID,
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
	messages.EXPECT().Revoke(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(domain.Audience(""), false, repository.ErrStaffMessageNotFound)

	err := service.RevokeStaffMessage(withEventToken(testEventPublicID), application.RevokeStaffMessageInput{
		TenantID:  testTenantPublicID,
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
		ListActive(gomock.Any(), testTenantPublicID, testEventPublicID, []domain.Audience{domain.AudienceStaff, domain.AudienceBoth}).
		Return(listed, nil)

	got, err := service.ListStaffMessages(withEventToken(testEventPublicID), application.ListStaffMessagesInput{
		TenantID: testTenantPublicID,
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
			TenantID: testTenantPublicID,
			EventID:  testEventPublicID,
			Audience: audience,
		})
		if !errors.Is(err, application.ErrGuestAudienceOnly) {
			t.Errorf("audience %q: ListStaffMessages error = %v, want %v", audience, err, application.ErrGuestAudienceOnly)
		}
	}

	service, messages := newStaffCommunication(t)
	messages.EXPECT().
		ListActive(gomock.Any(), testTenantPublicID, testEventPublicID, []domain.Audience{domain.AudienceGuest, domain.AudienceBoth}).
		Return(nil, nil)

	if _, err := service.ListStaffMessages(withServiceToken(), application.ListStaffMessagesInput{
		TenantID: testTenantPublicID,
		EventID:  testEventPublicID,
		Audience: domain.AudienceGuest,
	}); err != nil {
		t.Errorf("ListStaffMessages(GUEST) as a service: %v", err)
	}
}

func TestStaffCommunicationChecksTheRequestScope(t *testing.T) {
	t.Parallel()

	calls := map[string]func(*application.StaffCommunicationService, context.Context, string, string) error{
		"send": func(service *application.StaffCommunicationService, ctx context.Context, tenantID, eventID string) error {
			_, err := service.SendStaffMessage(ctx, application.SendStaffMessageInput{
				TenantID: tenantID, EventID: eventID, Audience: domain.AudienceStaff, Body: "hi",
			})

			return err
		},
		"revoke": func(service *application.StaffCommunicationService, ctx context.Context, tenantID, eventID string) error {
			return service.RevokeStaffMessage(ctx, application.RevokeStaffMessageInput{TenantID: tenantID, EventID: eventID, MessageID: testMessageID})
		},
		"share": func(service *application.StaffCommunicationService, ctx context.Context, tenantID, eventID string) error {
			_, err := service.ShareGuidanceInfo(ctx, application.ShareGuidanceInfoInput{TenantID: tenantID, EventID: eventID, Body: "hi"})

			return err
		},
		"list": func(service *application.StaffCommunicationService, ctx context.Context, tenantID, eventID string) error {
			_, err := service.ListStaffMessages(ctx, application.ListStaffMessagesInput{TenantID: tenantID, EventID: eventID, Audience: domain.AudienceStaff})

			return err
		},
	}

	tests := []struct {
		name     string
		tenantID string
		eventID  string
		want     error
	}{
		{"missing tenant", " ", testEventPublicID, domain.ErrTenantRequired},
		{"missing event", testTenantPublicID, " ", domain.ErrEventRequired},
		{"tenant other than the token's", otherTenantPublicID, testEventPublicID, tenantctx.ErrMismatch},
		{"event other than the token's", testTenantPublicID, otherEventPublicID, tenantctx.ErrEventMismatch},
	}

	for name, call := range calls {
		for _, tt := range tests {
			service, _ := newStaffCommunication(t)

			if err := call(service, withEventToken(testEventPublicID), tt.tenantID, tt.eventID); !errors.Is(err, tt.want) {
				t.Errorf("%s with %s: error = %v, want %v", name, tt.name, err, tt.want)
			}
		}
	}
}

func TestListStaffMessagesRequiresTheScopeFromServiceCallers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tenantID string
		eventID  string
		want     error
	}{
		{" ", testEventPublicID, domain.ErrTenantRequired},
		{testTenantPublicID, " ", domain.ErrEventRequired},
	}

	for _, tt := range tests {
		service, _ := newStaffCommunication(t)

		_, err := service.ListStaffMessages(withServiceToken(), application.ListStaffMessagesInput{
			TenantID: tt.tenantID,
			EventID:  tt.eventID,
			Audience: domain.AudienceGuest,
		})
		if !errors.Is(err, tt.want) {
			t.Errorf("ListStaffMessages(%q, %q) error = %v, want %v", tt.tenantID, tt.eventID, err, tt.want)
		}
	}
}

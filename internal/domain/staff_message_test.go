package domain_test

import (
	"errors"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

var publicIDPattern = regexp.MustCompile(`^[0-9a-f]{16}$`)

var sentAt = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

func TestNewStaffMessage(t *testing.T) {
	t.Parallel()

	message, err := domain.NewStaffMessage(" tenant ", " event ", " staff-1 ", domain.AudienceGuest, "  gate B is open  ", sentAt)
	if err != nil {
		t.Fatalf("NewStaffMessage: %v", err)
	}

	if !publicIDPattern.MatchString(string(message.ID)) {
		t.Errorf("message ID %q is not 16 lowercase hex characters", message.ID)
	}

	want := domain.StaffMessage{
		ID:              message.ID,
		TenantPublicID:  "tenant",
		EventID:         "event",
		SenderUserID:    "staff-1",
		Kind:            domain.MessageKindStaffMessage,
		Audience:        domain.AudienceGuest,
		Body:            "gate B is open",
		RelatedRouteIDs: []string{},
		SentAt:          sentAt,
	}

	assertMessage(t, message, want)
}

func TestNewStaffMessageRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		tenant   string
		event    string
		sender   string
		audience domain.Audience
		body     string
		want     error
	}{
		{"missing tenant", "", "event", "staff-1", domain.AudienceStaff, "hello", domain.ErrTenantRequired},
		{"missing event", "tenant", " ", "staff-1", domain.AudienceStaff, "hello", domain.ErrEventRequired},
		{"missing sender", "tenant", "event", " ", domain.AudienceStaff, "hello", domain.ErrSenderRequired},
		{"unspecified audience", "tenant", "event", "staff-1", "", "hello", domain.ErrAudienceRequired},
		{"blank body", "tenant", "event", "staff-1", domain.AudienceStaff, " \n\t ", domain.ErrBodyRequired},
		{"body over the limit", "tenant", "event", "staff-1", domain.AudienceStaff, strings.Repeat("あ", domain.MaxBodyLength+1), domain.ErrBodyTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := domain.NewStaffMessage(tt.tenant, tt.event, tt.sender, tt.audience, tt.body, sentAt)
			if !errors.Is(err, tt.want) {
				t.Errorf("NewStaffMessage error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNewStaffMessageAcceptsABodyAtTheLimitInRunes(t *testing.T) {
	t.Parallel()

	if _, err := domain.NewStaffMessage("tenant", "event", "staff-1", domain.AudienceStaff, strings.Repeat("あ", domain.MaxBodyLength), sentAt); err != nil {
		t.Errorf("NewStaffMessage: %v", err)
	}
}

func TestNewGuidanceInfo(t *testing.T) {
	t.Parallel()

	message, err := domain.NewGuidanceInfo("tenant", "event", "staff-1", "one way from A to B", []string{" route-a ", "route-b"}, sentAt)
	if err != nil {
		t.Fatalf("NewGuidanceInfo: %v", err)
	}

	assertMessage(t, message, domain.StaffMessage{
		ID:              message.ID,
		TenantPublicID:  "tenant",
		EventID:         "event",
		SenderUserID:    "staff-1",
		Kind:            domain.MessageKindGuidanceInfo,
		Audience:        domain.AudienceStaff,
		Body:            "one way from A to B",
		RelatedRouteIDs: []string{"route-a", "route-b"},
		SentAt:          sentAt,
	})
}

func TestNewGuidanceInfoRejectsABlankRouteID(t *testing.T) {
	t.Parallel()

	_, err := domain.NewGuidanceInfo("tenant", "event", "staff-1", "one way", []string{"route-a", " "}, sentAt)
	if !errors.Is(err, domain.ErrRouteIDRequired) {
		t.Errorf("NewGuidanceInfo error = %v, want %v", err, domain.ErrRouteIDRequired)
	}
}

func TestAudiencesVisibleTo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		audience domain.Audience
		want     []domain.Audience
	}{
		{domain.AudienceGuest, []domain.Audience{domain.AudienceGuest, domain.AudienceBoth}},
		{domain.AudienceStaff, []domain.Audience{domain.AudienceStaff, domain.AudienceBoth}},
		{domain.AudienceBoth, []domain.Audience{domain.AudienceBoth}},
		{"", []domain.Audience{domain.AudienceStaff, domain.AudienceGuest, domain.AudienceBoth}},
	}

	for _, tt := range tests {
		if got := domain.AudiencesVisibleTo(tt.audience); !slices.Equal(got, tt.want) {
			t.Errorf("AudiencesVisibleTo(%q) = %v, want %v", tt.audience, got, tt.want)
		}
	}
}

func TestParseMessageID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value string
		want  error
	}{
		{"0123456789abcdef", nil},
		{"0123456789ABCDEF", domain.ErrInvalidPublicID},
		{"0123456789abcde", domain.ErrInvalidPublicID},
		{"0123456789abcdeg", domain.ErrInvalidPublicID},
		{"", domain.ErrInvalidPublicID},
	}

	for _, tt := range tests {
		_, err := domain.ParseMessageID(tt.value)
		if !errors.Is(err, tt.want) {
			t.Errorf("ParseMessageID(%q) error = %v, want %v", tt.value, err, tt.want)
		}
	}
}

func assertMessage(t *testing.T, got, want domain.StaffMessage) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("message = %+v, want %+v", got, want)
	}
}

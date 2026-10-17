package domain_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

func connection(userID, deviceID string, connected bool) domain.ConnectionState {
	return domain.ConnectionState{
		TenantPublicID: "a1b2c3d4e5f60718",
		EventID:        "0f1e2d3c4b5a6978",
		UserID:         userID,
		DeviceID:       deviceID,
		Connected:      connected,
		At:             time.Date(2026, time.October, 17, 9, 0, 0, 0, time.UTC),
	}
}

func TestSelectRecipientsPicksEveryStaffMemberWithAConnectionStateOnce(t *testing.T) {
	t.Parallel()

	got := domain.SelectRecipients([]domain.ConnectionState{
		connection("staff-b", "phone", false),
		connection("staff-a", "phone", true),
		connection("staff-b", "tablet", true),
	})

	if want := []string{"staff-a", "staff-b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("SelectRecipients = %v, want %v", got, want)
	}
}

func TestRouteDeliveryRoutesOnlyTheSelectedRecipients(t *testing.T) {
	t.Parallel()

	states := []domain.ConnectionState{
		connection("staff-a", "phone", true),
		connection("staff-b", "phone", false),
		connection("staff-c", "phone", true),
	}

	got := domain.RouteDelivery([]string{"staff-b", "staff-c", "staff-without-state"}, states)

	want := domain.DeliveryRoute{Realtime: []string{"staff-c"}, Push: []string{"staff-b", "staff-without-state"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RouteDelivery = %+v, want %+v without staff-a, who was not selected", got, want)
	}
}

func TestNewProposalRequiresAKnownKindWithContent(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		kind    domain.ProposalKind
		content string
	}{
		{"", `{}`},
		{"staff_message", `{}`},
		{domain.ProposalLineGuidance, ``},
	} {
		if _, err := domain.NewProposal(tt.kind, []byte(tt.content)); err == nil {
			t.Errorf("NewProposal(%q, %q) succeeded, want %v", tt.kind, tt.content, domain.ErrProposalRequired)
		}
	}

	if _, err := domain.NewProposal(domain.ProposalLineGuidance, []byte(`{}`)); err != nil {
		t.Errorf("NewProposal(line_guidance, {}) = %v, want an empty guidance accepted", err)
	}
}

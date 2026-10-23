package domain_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	operationv1 "github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1"
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

func TestNewProposalRequiresAFlowProposalOrLineGuidance(t *testing.T) {
	t.Parallel()

	for _, payload := range []*operationv1.DeliveryPayload{
		nil,
		{},
		{Kind: &operationv1.DeliveryPayload_StaffMessage{StaffMessage: &operationv1.StaffMessage{}}},
		{Kind: &operationv1.DeliveryPayload_LineGuidance{}},
	} {
		if _, err := domain.NewProposal(payload); !errors.Is(err, domain.ErrProposalRequired) {
			t.Errorf("NewProposal(%v) = %v, want %v", payload, err, domain.ErrProposalRequired)
		}
	}

	for want, payload := range map[domain.ProposalKind]*operationv1.DeliveryPayload{
		domain.ProposalFlowProposals: {Kind: &operationv1.DeliveryPayload_FlowProposals{FlowProposals: &operationv1.FlowProposals{}}},
		domain.ProposalLineGuidance:  {Kind: &operationv1.DeliveryPayload_LineGuidance{LineGuidance: &operationv1.LineGuidance{}}},
	} {
		proposal, err := domain.NewProposal(payload)
		if err != nil || proposal.Kind != want {
			t.Errorf("NewProposal(%v) = %q, %v, want an empty %s accepted", payload, proposal.Kind, err, want)
		}
	}
}

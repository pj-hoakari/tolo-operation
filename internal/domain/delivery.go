package domain

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/google/uuid"

	operationv1 "github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1"
)

var ErrProposalRequired = errors.New("proposal payload is required")

type DeliveryID string

func NewDeliveryID() (DeliveryID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate delivery ID: %w", err)
	}

	return DeliveryID(id.String()), nil
}

type ProposalKind string

const (
	ProposalFlowProposals ProposalKind = "flow_proposals"
	ProposalLineGuidance  ProposalKind = "line_guidance"
)

var proposalPushTitles = map[ProposalKind]string{
	ProposalFlowProposals: "誘導提案が届きました",
	ProposalLineGuidance:  "行列案内が届きました",
}

type Proposal struct {
	Kind    ProposalKind
	Payload *operationv1.DeliveryPayload
}

func NewProposal(payload *operationv1.DeliveryPayload) (Proposal, error) {
	switch {
	case payload.GetFlowProposals() != nil:
		return Proposal{Kind: ProposalFlowProposals, Payload: payload}, nil
	case payload.GetLineGuidance() != nil:
		return Proposal{Kind: ProposalLineGuidance, Payload: payload}, nil
	default:
		return Proposal{}, ErrProposalRequired
	}
}

type PushMessage struct {
	Title string
	Body  string
	Data  map[string]string
}

func ProposalArrival(eventID string, id DeliveryID, proposal Proposal) PushMessage {
	return PushMessage{
		Title: proposalPushTitles[proposal.Kind],
		Body:  "",
		Data: map[string]string{
			"kind":        string(proposal.Kind),
			"event_id":    eventID,
			"delivery_id": string(id),
		},
	}
}

func SelectRecipients(states []ConnectionState) []string {
	users := map[string]struct{}{}
	for _, state := range states {
		users[state.UserID] = struct{}{}
	}

	return slices.Sorted(maps.Keys(users))
}

type DeliveryRoute struct {
	Realtime []string
	Push     []string
}

func RouteDelivery(recipients []string, states []ConnectionState) DeliveryRoute {
	connected := map[string]bool{}
	for _, state := range states {
		connected[state.UserID] = connected[state.UserID] || state.Connected
	}

	route := DeliveryRoute{Realtime: nil, Push: nil}

	for _, userID := range recipients {
		if connected[userID] {
			route.Realtime = append(route.Realtime, userID)
		} else {
			route.Push = append(route.Push, userID)
		}
	}

	return route
}

package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/google/uuid"
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
	Content json.RawMessage
}

func NewProposal(kind ProposalKind, content json.RawMessage) (Proposal, error) {
	if _, ok := proposalPushTitles[kind]; !ok || len(content) == 0 {
		return Proposal{}, ErrProposalRequired
	}

	return Proposal{Kind: kind, Content: content}, nil
}

func (p Proposal) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[ProposalKind]json.RawMessage{p.Kind: p.Content})
}

type DeliveryOrder struct {
	DeliveryID       DeliveryID `json:"delivery_id"`
	EventID          string     `json:"event_id"`
	RecipientUserIDs []string   `json:"recipient_user_ids"`
	Payload          Proposal   `json:"payload"`
}

func (DeliveryOrder) Topic() Topic { return TopicRealtimeDelivery }

func (o DeliveryOrder) OrderingKey() string { return o.EventID }

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

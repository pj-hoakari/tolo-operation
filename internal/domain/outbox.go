package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

type Topic string

const TopicGuestMessages Topic = "guest-messages"

type OutboxPayload interface {
	Topic() Topic
	OrderingKey() string
}

type OutboxMessage struct {
	Topic       Topic
	OrderingKey string
	Payload     json.RawMessage
}

func NewOutboxMessage(payload OutboxPayload) (OutboxMessage, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return OutboxMessage{}, fmt.Errorf("encode %s payload: %w", payload.Topic(), err)
	}

	return OutboxMessage{Topic: payload.Topic(), OrderingKey: payload.OrderingKey(), Payload: encoded}, nil
}

type OutboxStatus string

const (
	OutboxPending   OutboxStatus = "pending"
	OutboxSent      OutboxStatus = "sent"
	OutboxAbandoned OutboxStatus = "abandoned"
)

type OutboxEntry struct {
	ID       int64
	Message  OutboxMessage
	Status   OutboxStatus
	Attempts int
}

func (e OutboxEntry) Published() OutboxEntry {
	e.Attempts++
	e.Status = OutboxSent

	return e
}

func (e OutboxEntry) PublishFailed(maxAttempts int) OutboxEntry {
	e.Attempts++
	if e.Attempts >= maxAttempts {
		e.Status = OutboxAbandoned
	}

	return e
}

type GuestMessagesUpdate struct {
	EventID           string         `json:"event_id"`
	Upserts           []GuestMessage `json:"upserts,omitempty"`
	RevokedMessageIDs []string       `json:"revoked_message_ids,omitempty"`
}

type GuestMessage struct {
	MessageID string    `json:"message_id"`
	Body      string    `json:"body"`
	SentAt    time.Time `json:"sent_at"`
}

func (GuestMessagesUpdate) Topic() Topic { return TopicGuestMessages }

func (u GuestMessagesUpdate) OrderingKey() string { return u.EventID }

func (a Audience) IncludesGuests() bool {
	return a == AudienceGuest || a == AudienceBoth
}

func GuestUpsert(message StaffMessage) *GuestMessagesUpdate {
	if !message.Audience.IncludesGuests() {
		return nil
	}

	return &GuestMessagesUpdate{
		EventID: message.EventID,
		Upserts: []GuestMessage{{
			MessageID: string(message.ID),
			Body:      message.Body,
			SentAt:    message.SentAt.UTC(),
		}},
		RevokedMessageIDs: nil,
	}
}

func GuestRevocation(eventID string, id MessageID, audience Audience) *GuestMessagesUpdate {
	if !audience.IncludesGuests() {
		return nil
	}

	return &GuestMessagesUpdate{EventID: eventID, Upserts: nil, RevokedMessageIDs: []string{string(id)}}
}

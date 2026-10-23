package domain

import (
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	operationv1 "github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1"
)

type Topic string

const (
	TopicGuestMessages    Topic = "guest-messages"
	TopicRealtimeDelivery Topic = "realtime-delivery"
)

type OutboxMessage struct {
	Topic       Topic
	OrderingKey string
	Payload     []byte
}

func NewOutboxMessage(topic Topic, orderingKey string, payload proto.Message) (OutboxMessage, error) {
	encoded, err := proto.Marshal(payload)
	if err != nil {
		return OutboxMessage{}, fmt.Errorf("encode %s payload: %w", topic, err)
	}

	return OutboxMessage{Topic: topic, OrderingKey: orderingKey, Payload: encoded}, nil
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

func (a Audience) IncludesGuests() bool {
	return a == AudienceGuest || a == AudienceBoth
}

func GuestUpsert(message StaffMessage) *operationv1.GuestMessagesUpdate {
	if !message.Audience.IncludesGuests() {
		return nil
	}

	return &operationv1.GuestMessagesUpdate{
		EventId: message.EventID,
		Upserts: []*operationv1.GuestMessage{{
			MessageId: string(message.ID),
			Body:      message.Body,
			SentAt:    timestamppb.New(message.SentAt.UTC()),
		}},
		RevokedMessageIds: nil,
	}
}

func GuestRevocation(eventID string, id MessageID, audience Audience) *operationv1.GuestMessagesUpdate {
	if !audience.IncludesGuests() {
		return nil
	}

	return &operationv1.GuestMessagesUpdate{EventId: eventID, Upserts: nil, RevokedMessageIds: []string{string(id)}}
}

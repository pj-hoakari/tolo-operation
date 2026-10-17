package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrSenderRequired   = errors.New("sender user ID is required")
	ErrAudienceRequired = errors.New("audience is required")
	ErrBodyRequired     = errors.New("body is required")
	ErrBodyTooLong      = errors.New("body is too long")
	ErrRouteIDRequired  = errors.New("related route ID is required")
)

const MaxBodyLength = 2000

type MessageID string

func NewMessageID() (MessageID, error) {
	id, err := newPublicID()
	if err != nil {
		return "", err
	}

	return MessageID(id), nil
}

func ParseMessageID(value string) (MessageID, error) {
	id, err := parsePublicID(value)
	if err != nil {
		return "", err
	}

	return MessageID(id), nil
}

type Audience string

const (
	AudienceStaff Audience = "staff"
	AudienceGuest Audience = "guest"
	AudienceBoth  Audience = "both"
)

func AudiencesVisibleTo(audience Audience) []Audience {
	switch audience {
	case AudienceStaff:
		return []Audience{AudienceStaff, AudienceBoth}
	case AudienceGuest:
		return []Audience{AudienceGuest, AudienceBoth}
	case AudienceBoth:
		return []Audience{AudienceBoth}
	default:
		return []Audience{AudienceStaff, AudienceGuest, AudienceBoth}
	}
}

type MessageKind string

const (
	MessageKindStaffMessage MessageKind = "staff_message"
	MessageKindGuidanceInfo MessageKind = "guidance_info"
)

type StaffMessage struct {
	ID              MessageID
	TenantPublicID  string
	EventID         string
	SenderUserID    string
	Kind            MessageKind
	Audience        Audience
	Body            string
	RelatedRouteIDs []string
	SentAt          time.Time
}

func NewStaffMessage(tenantPublicID, eventID, senderUserID string, audience Audience, body string, sentAt time.Time) (StaffMessage, error) {
	if audience == "" {
		return StaffMessage{}, ErrAudienceRequired
	}

	return newStaffMessage(tenantPublicID, eventID, senderUserID, MessageKindStaffMessage, audience, body, []string{}, sentAt)
}

func NewGuidanceInfo(tenantPublicID, eventID, senderUserID, body string, relatedRouteIDs []string, sentAt time.Time) (StaffMessage, error) {
	routeIDs := make([]string, 0, len(relatedRouteIDs))

	for _, routeID := range relatedRouteIDs {
		routeID = strings.TrimSpace(routeID)
		if routeID == "" {
			return StaffMessage{}, ErrRouteIDRequired
		}

		routeIDs = append(routeIDs, routeID)
	}

	return newStaffMessage(tenantPublicID, eventID, senderUserID, MessageKindGuidanceInfo, AudienceStaff, body, routeIDs, sentAt)
}

func newStaffMessage(
	tenantPublicID, eventID, senderUserID string,
	kind MessageKind,
	audience Audience,
	body string,
	relatedRouteIDs []string,
	sentAt time.Time,
) (StaffMessage, error) {
	tenantPublicID, eventID, err := requireScope(tenantPublicID, eventID)
	if err != nil {
		return StaffMessage{}, err
	}

	senderUserID = strings.TrimSpace(senderUserID)
	if senderUserID == "" {
		return StaffMessage{}, ErrSenderRequired
	}

	body = strings.TrimSpace(body)
	if body == "" {
		return StaffMessage{}, ErrBodyRequired
	}

	if utf8.RuneCountInString(body) > MaxBodyLength {
		return StaffMessage{}, ErrBodyTooLong
	}

	id, err := NewMessageID()
	if err != nil {
		return StaffMessage{}, err
	}

	return StaffMessage{
		ID:              id,
		TenantPublicID:  tenantPublicID,
		EventID:         eventID,
		SenderUserID:    senderUserID,
		Kind:            kind,
		Audience:        audience,
		Body:            body,
		RelatedRouteIDs: relatedRouteIDs,
		SentAt:          sentAt,
	}, nil
}

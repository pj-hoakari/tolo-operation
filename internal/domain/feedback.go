package domain

import (
	"errors"
	"math"
	"strings"
	"time"
)

var (
	ErrComputedAtRequired     = errors.New("feedback computed_at is required")
	ErrFeedbackKindRequired   = errors.New("feedback kind is required")
	ErrFeedbackValueNotFinite = errors.New("feedback value must be finite")
)

type FeedbackEntry struct {
	AttributeTag string
	Kind         string
	Value        float64
}

type FeedbackValues struct {
	TenantPublicID string
	EventID        string
	ComputedAt     time.Time
	Entries        []FeedbackEntry
}

func NewFeedbackValues(tenantPublicID, eventID string, computedAt time.Time, entries []FeedbackEntry) (FeedbackValues, error) {
	tenantPublicID, eventID, err := requireScope(tenantPublicID, eventID)
	if err != nil {
		return FeedbackValues{}, err
	}

	if computedAt.IsZero() {
		return FeedbackValues{}, ErrComputedAtRequired
	}

	validated := make([]FeedbackEntry, 0, len(entries))

	for _, entry := range entries {
		entry.AttributeTag = strings.TrimSpace(entry.AttributeTag)

		entry.Kind = strings.TrimSpace(entry.Kind)
		if entry.Kind == "" {
			return FeedbackValues{}, ErrFeedbackKindRequired
		}

		if math.IsNaN(entry.Value) || math.IsInf(entry.Value, 0) {
			return FeedbackValues{}, ErrFeedbackValueNotFinite
		}

		validated = append(validated, entry)
	}

	return FeedbackValues{TenantPublicID: tenantPublicID, EventID: eventID, ComputedAt: computedAt, Entries: validated}, nil
}

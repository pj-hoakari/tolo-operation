package domain_test

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

func TestNewFeedbackValues(t *testing.T) {
	t.Parallel()

	values, err := domain.NewFeedbackValues(" tenant ", "event", sentAt, []domain.FeedbackEntry{
		{AttributeTag: "", Kind: " objective ", Value: 0.75},
		{AttributeTag: " family ", Kind: "wait_minutes", Value: -2},
	})
	if err != nil {
		t.Fatalf("NewFeedbackValues: %v", err)
	}

	want := domain.FeedbackValues{TenantPublicID: "tenant", EventID: "event", ComputedAt: sentAt, Entries: []domain.FeedbackEntry{
		{AttributeTag: "", Kind: "objective", Value: 0.75},
		{AttributeTag: "family", Kind: "wait_minutes", Value: -2},
	}}
	if !reflect.DeepEqual(values, want) {
		t.Errorf("NewFeedbackValues = %+v, want %+v", values, want)
	}
}

func TestNewFeedbackValuesRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tenant     string
		event      string
		computedAt time.Time
		entry      domain.FeedbackEntry
		want       error
	}{
		{"missing tenant", " ", "event", sentAt, domain.FeedbackEntry{Kind: "objective"}, domain.ErrTenantRequired},
		{"missing event", "tenant", " ", sentAt, domain.FeedbackEntry{Kind: "objective"}, domain.ErrEventRequired},
		{"missing computed_at", "tenant", "event", time.Time{}, domain.FeedbackEntry{Kind: "objective"}, domain.ErrComputedAtRequired},
		{"blank kind", "tenant", "event", sentAt, domain.FeedbackEntry{Kind: " "}, domain.ErrFeedbackKindRequired},
		{"NaN", "tenant", "event", sentAt, domain.FeedbackEntry{Kind: "objective", Value: math.NaN()}, domain.ErrFeedbackValueNotFinite},
		{"infinity", "tenant", "event", sentAt, domain.FeedbackEntry{Kind: "objective", Value: math.Inf(-1)}, domain.ErrFeedbackValueNotFinite},
	}

	for _, tt := range tests {
		if _, err := domain.NewFeedbackValues(tt.tenant, tt.event, tt.computedAt, []domain.FeedbackEntry{tt.entry}); !errors.Is(err, tt.want) {
			t.Errorf("%s: NewFeedbackValues error = %v, want %v", tt.name, err, tt.want)
		}
	}
}

package db

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

func storedFeedback(t *testing.T, tenantPublicID, eventID string) []domain.FeedbackEntry {
	t.Helper()

	var rows []struct {
		AttributeTag string  `db:"attribute_tag"`
		Kind         string  `db:"kind"`
		Value        float64 `db:"value"`
	}

	if err := testDB.SelectContext(context.Background(), &rows, `
		SELECT attribute_tag, kind, value FROM feedback_values
		WHERE tenant_public_id = $1 AND event_id = $2 ORDER BY computed_at, attribute_tag, kind`, tenantPublicID, eventID); err != nil {
		t.Fatalf("select feedback values: %v", err)
	}

	entries := make([]domain.FeedbackEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, domain.FeedbackEntry{AttributeTag: row.AttributeTag, Kind: row.Kind, Value: row.Value})
	}

	return entries
}

func recordFeedback(
	t *testing.T,
	repo *PostgresFeedbackRepository,
	tenantPublicID, eventID string,
	computedAt time.Time,
	entries ...domain.FeedbackEntry,
) {
	t.Helper()

	values, err := domain.NewFeedbackValues(tenantPublicID, eventID, computedAt, entries)
	if err != nil {
		t.Fatalf("NewFeedbackValues: %v", err)
	}

	if err := repo.Record(context.Background(), values); err != nil {
		t.Fatalf("Record: %v", err)
	}
}

func TestPostgresFeedbackRepositoryIgnoresAResentBatch(t *testing.T) {
	repo := NewPostgresFeedbackRepository(testDB)
	tenant, event := "tenant-feedback-resend", "event-feedback-resend"
	batch := []domain.FeedbackEntry{
		{AttributeTag: "", Kind: "objective", Value: 0.75},
		{AttributeTag: "family", Kind: "wait_minutes", Value: 4},
	}

	recordFeedback(t, repo, tenant, event, baseTime, batch...)
	recordFeedback(t, repo, tenant, event, baseTime, batch...)
	recordFeedback(t, repo, tenant, event, baseTime.Add(time.Minute), domain.FeedbackEntry{Kind: "objective", Value: 0.5})

	want := []domain.FeedbackEntry{
		{AttributeTag: "", Kind: "objective", Value: 0.75},
		{AttributeTag: "family", Kind: "wait_minutes", Value: 4},
		{AttributeTag: "", Kind: "objective", Value: 0.5},
	}
	if got := storedFeedback(t, tenant, event); !reflect.DeepEqual(got, want) {
		t.Errorf("stored feedback = %+v, want %+v", got, want)
	}
}

func TestPostgresFeedbackRepositoryRecordsAnEmptyBatch(t *testing.T) {
	repo := NewPostgresFeedbackRepository(testDB)

	recordFeedback(t, repo, "tenant-feedback-empty", "event-feedback-empty", baseTime)

	if got := storedFeedback(t, "tenant-feedback-empty", "event-feedback-empty"); len(got) != 0 {
		t.Errorf("stored feedback = %+v, want none", got)
	}
}

func TestPostgresFeedbackRepositoryKeepsTenantsApartOnTheSameEvent(t *testing.T) {
	repo := NewPostgresFeedbackRepository(testDB)
	event := "event-feedback-shared"
	first, second := "tenant-feedback-first", "tenant-feedback-second"

	recordFeedback(t, repo, first, event, baseTime, domain.FeedbackEntry{Kind: "objective", Value: 0.75})
	recordFeedback(t, repo, second, event, baseTime, domain.FeedbackEntry{Kind: "objective", Value: 0.25})
	recordFeedback(t, repo, first, event, baseTime, domain.FeedbackEntry{Kind: "objective", Value: 0.75})
	recordFeedback(t, repo, second, event, baseTime, domain.FeedbackEntry{Kind: "objective", Value: 0.25})

	for tenant, want := range map[string][]domain.FeedbackEntry{
		first:  {{Kind: "objective", Value: 0.75}},
		second: {{Kind: "objective", Value: 0.25}},
	} {
		if got := storedFeedback(t, tenant, event); !reflect.DeepEqual(got, want) {
			t.Errorf("stored feedback of %s = %+v, want %+v", tenant, got, want)
		}
	}
}

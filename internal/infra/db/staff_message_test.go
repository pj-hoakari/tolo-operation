package db

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
)

var baseTime = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

func createMessage(
	t *testing.T,
	repo *PostgresStaffMessageRepository,
	tenantPublicID, eventID string,
	audience domain.Audience,
	sentAt time.Time,
) domain.StaffMessage {
	t.Helper()

	message, err := domain.NewStaffMessage(tenantPublicID, eventID, "staff-1", audience, "message to "+string(audience), sentAt)
	if err != nil {
		t.Fatalf("NewStaffMessage: %v", err)
	}

	if err := repo.Create(context.Background(), message); err != nil {
		t.Fatalf("Create: %v", err)
	}

	return message
}

func messageIDs(messages []domain.StaffMessage) []domain.MessageID {
	ids := make([]domain.MessageID, 0, len(messages))
	for _, message := range messages {
		ids = append(ids, message.ID)
	}

	return ids
}

func TestPostgresStaffMessageRepositoryRoundTrip(t *testing.T) {
	repo := NewPostgresStaffMessageRepository(testDB)
	ctx := context.Background()

	guidance, err := domain.NewGuidanceInfo("tenant-message-round-trip", "event-message-round-trip", "staff-1", "one way", []string{"route-a", "route-b"}, baseTime)
	if err != nil {
		t.Fatalf("NewGuidanceInfo: %v", err)
	}

	if err := repo.Create(ctx, guidance); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.ListActive(ctx, "tenant-message-round-trip", "event-message-round-trip", domain.AudiencesVisibleTo(""))
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("ListActive returned %d messages, want 1", len(got))
	}

	got[0].SentAt = got[0].SentAt.UTC()
	if !reflect.DeepEqual(got[0], guidance) {
		t.Errorf("ListActive = %+v, want %+v", got[0], guidance)
	}
}

func TestPostgresStaffMessageRepositoryListsByAudienceInSendOrder(t *testing.T) {
	repo := NewPostgresStaffMessageRepository(testDB)
	tenant, event := "tenant-message-audience", "event-message-audience"

	both := createMessage(t, repo, tenant, event, domain.AudienceBoth, baseTime.Add(3*time.Minute))
	guest := createMessage(t, repo, tenant, event, domain.AudienceGuest, baseTime.Add(2*time.Minute))
	staff := createMessage(t, repo, tenant, event, domain.AudienceStaff, baseTime.Add(time.Minute))
	createMessage(t, repo, tenant, "event-message-audience-other", domain.AudienceBoth, baseTime)

	tests := []struct {
		filter domain.Audience
		want   []domain.MessageID
	}{
		{domain.AudienceGuest, []domain.MessageID{guest.ID, both.ID}},
		{domain.AudienceStaff, []domain.MessageID{staff.ID, both.ID}},
		{"", []domain.MessageID{staff.ID, guest.ID, both.ID}},
	}

	for _, tt := range tests {
		got, err := repo.ListActive(context.Background(), tenant, event, domain.AudiencesVisibleTo(tt.filter))
		if err != nil {
			t.Fatalf("ListActive(%q): %v", tt.filter, err)
		}

		if ids := messageIDs(got); !reflect.DeepEqual(ids, tt.want) {
			t.Errorf("ListActive(%q) = %v, want %v", tt.filter, ids, tt.want)
		}
	}
}

func TestPostgresStaffMessageRepositoryRevoke(t *testing.T) {
	repo := NewPostgresStaffMessageRepository(testDB)
	tenant, event := "tenant-message-revoke", "event-message-revoke"
	ctx := context.Background()

	revoked := createMessage(t, repo, tenant, event, domain.AudienceGuest, baseTime)
	kept := createMessage(t, repo, tenant, event, domain.AudienceGuest, baseTime.Add(time.Minute))

	if err := repo.Revoke(ctx, tenant, event, revoked.ID, baseTime.Add(time.Hour)); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	if err := repo.Revoke(ctx, tenant, event, revoked.ID, baseTime.Add(2*time.Hour)); err != nil {
		t.Fatalf("Revoke again: %v", err)
	}

	var revokedAt time.Time
	if err := testDB.GetContext(ctx, &revokedAt, `SELECT revoked_at FROM staff_messages WHERE message_id = $1`, string(revoked.ID)); err != nil {
		t.Fatalf("select revoked_at: %v", err)
	}

	if !revokedAt.Equal(baseTime.Add(time.Hour)) {
		t.Errorf("revoked_at = %v, want the first revocation %v", revokedAt, baseTime.Add(time.Hour))
	}

	got, err := repo.ListActive(ctx, tenant, event, domain.AudiencesVisibleTo(""))
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}

	if ids := messageIDs(got); !reflect.DeepEqual(ids, []domain.MessageID{kept.ID}) {
		t.Errorf("ListActive = %v, want only %v", ids, kept.ID)
	}
}

func TestPostgresStaffMessageRepositoryRevokeIsScopedToTenantAndEvent(t *testing.T) {
	repo := NewPostgresStaffMessageRepository(testDB)
	tenant, event := "tenant-message-revoke-scope", "event-message-revoke-scope"
	message := createMessage(t, repo, tenant, event, domain.AudienceStaff, baseTime)

	unknown, err := domain.NewMessageID()
	if err != nil {
		t.Fatalf("NewMessageID: %v", err)
	}

	tests := []struct {
		name   string
		tenant string
		event  string
		id     domain.MessageID
	}{
		{"unknown message", tenant, event, unknown},
		{"other event", tenant, "event-message-revoke-scope-other", message.ID},
		{"other tenant", "tenant-message-revoke-scope-other", event, message.ID},
	}

	for _, tt := range tests {
		err := repo.Revoke(context.Background(), tt.tenant, tt.event, tt.id, baseTime)
		if !errors.Is(err, repository.ErrStaffMessageNotFound) {
			t.Errorf("%s: Revoke error = %v, want %v", tt.name, err, repository.ErrStaffMessageNotFound)
		}
	}

	got, err := repo.ListActive(context.Background(), tenant, event, domain.AudiencesVisibleTo(""))
	if err != nil {
		t.Fatalf("ListActive: %v", err)
	}

	if len(got) != 1 {
		t.Errorf("ListActive returned %d messages, want the message still active", len(got))
	}
}

func TestPostgresStaffMessageRepositoryListsOnlyTheRequestedTenant(t *testing.T) {
	repo := NewPostgresStaffMessageRepository(testDB)
	event := "event-message-shared"
	first := createMessage(t, repo, "tenant-message-first", event, domain.AudienceGuest, baseTime)
	second := createMessage(t, repo, "tenant-message-second", event, domain.AudienceGuest, baseTime.Add(time.Minute))

	for tenant, want := range map[string][]domain.MessageID{
		"tenant-message-first":  {first.ID},
		"tenant-message-second": {second.ID},
		"tenant-message-none":   {},
	} {
		got, err := repo.ListActive(context.Background(), tenant, event, domain.AudiencesVisibleTo(domain.AudienceGuest))
		if err != nil {
			t.Fatalf("ListActive(%s): %v", tenant, err)
		}

		if ids := messageIDs(got); !reflect.DeepEqual(ids, want) {
			t.Errorf("ListActive(%s) = %v, want %v", tenant, ids, want)
		}
	}
}

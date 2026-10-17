package db

import (
	"context"
	"testing"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

type connectionStateRow struct {
	Connected bool      `db:"connected"`
	At        time.Time `db:"at"`
}

func saveConnectionState(t *testing.T, repo *PostgresConnectionStateRepository, tenant, device string, connected bool, at time.Time) {
	t.Helper()

	state, err := domain.NewConnectionState(tenant, "event-connection", "staff-1", device, connected, at)
	if err != nil {
		t.Fatalf("NewConnectionState: %v", err)
	}

	if err := repo.Save(context.Background(), state); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

func storedConnectionState(t *testing.T, tenant, device string) connectionStateRow {
	t.Helper()

	var row connectionStateRow
	if err := testDB.GetContext(context.Background(), &row, `
		SELECT connected, at FROM connection_states
		WHERE tenant_public_id = $1 AND event_id = 'event-connection' AND user_id = 'staff-1' AND device_id = $2`,
		tenant, device); err != nil {
		t.Fatalf("select connection state: %v", err)
	}

	return row
}

func TestPostgresConnectionStateRepositoryKeepsTheLatestState(t *testing.T) {
	repo := NewPostgresConnectionStateRepository(testDB)
	tenant := "tenant-connection-order"

	saveConnectionState(t, repo, tenant, "phone-1", true, baseTime)
	saveConnectionState(t, repo, tenant, "phone-1", false, baseTime.Add(time.Minute))
	saveConnectionState(t, repo, tenant, "phone-1", true, baseTime.Add(30*time.Second))
	saveConnectionState(t, repo, tenant, "phone-1", true, baseTime.Add(time.Minute))

	got := storedConnectionState(t, tenant, "phone-1")
	if got.Connected || !got.At.Equal(baseTime.Add(time.Minute)) {
		t.Errorf("stored state = %+v, want disconnected at %v despite the late and the same-instant updates", got, baseTime.Add(time.Minute))
	}

	saveConnectionState(t, repo, tenant, "phone-1", true, baseTime.Add(2*time.Minute))

	got = storedConnectionState(t, tenant, "phone-1")
	if !got.Connected || !got.At.Equal(baseTime.Add(2*time.Minute)) {
		t.Errorf("stored state = %+v, want connected at %v", got, baseTime.Add(2*time.Minute))
	}
}

func TestPostgresConnectionStateRepositoryKeysByDeviceAndTenant(t *testing.T) {
	repo := NewPostgresConnectionStateRepository(testDB)

	saveConnectionState(t, repo, "tenant-connection-key", "phone-1", true, baseTime)
	saveConnectionState(t, repo, "tenant-connection-key", "tablet-1", false, baseTime.Add(time.Minute))
	saveConnectionState(t, repo, "tenant-connection-key-other", "phone-1", false, baseTime.Add(time.Minute))

	if got := storedConnectionState(t, "tenant-connection-key", "phone-1"); !got.Connected {
		t.Errorf("phone-1 state = %+v, want it left connected by the other device and tenant", got)
	}
}

func TestPostgresConnectionStateRepositoryListsOnlyTheRequestedTenantAndEvent(t *testing.T) {
	repo := NewPostgresConnectionStateRepository(testDB)
	ctx := context.Background()
	tenant, event := "tenant-connection-list", "event-connection-list"

	for _, s := range []struct{ tenant, event, user string }{
		{tenant, event, "staff-b"},
		{tenant, event, "staff-a"},
		{tenant, "event-connection-list-other", "staff-c"},
		{"tenant-connection-list-other", event, "staff-d"},
	} {
		state, err := domain.NewConnectionState(s.tenant, s.event, s.user, "phone", true, baseTime)
		if err != nil {
			t.Fatalf("NewConnectionState: %v", err)
		}

		if err := repo.Save(ctx, state); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	states, err := repo.ListByEvent(ctx, tenant, event)
	if err != nil {
		t.Fatalf("ListByEvent: %v", err)
	}

	var users []string
	for _, state := range states {
		users = append(users, state.UserID)

		if state.TenantPublicID != tenant || state.EventID != event || !state.Connected || !state.At.Equal(baseTime) {
			t.Errorf("state = %+v, want a connected state of the requested pair", state)
		}
	}

	if len(users) != 2 || users[0] != "staff-a" || users[1] != "staff-b" {
		t.Errorf("ListByEvent users = %v, want [staff-a staff-b]", users)
	}

	none, err := repo.ListByEvent(ctx, "tenant-connection-list-other", "event-connection-list-other")
	if err != nil || len(none) != 0 {
		t.Errorf("ListByEvent for a mismatched pair = %v, %v, want no states", none, err)
	}
}

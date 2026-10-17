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

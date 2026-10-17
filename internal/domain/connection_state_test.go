package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

func TestNewConnectionState(t *testing.T) {
	t.Parallel()

	state, err := domain.NewConnectionState("tenant", "event", " staff-1 ", " phone-1 ", true, sentAt)
	if err != nil {
		t.Fatalf("NewConnectionState: %v", err)
	}

	want := domain.ConnectionState{TenantPublicID: "tenant", EventID: "event", UserID: "staff-1", DeviceID: "phone-1", Connected: true, At: sentAt}
	if state != want {
		t.Errorf("NewConnectionState = %+v, want %+v", state, want)
	}
}

func TestNewConnectionStateRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		user   string
		device string
		at     time.Time
		want   error
	}{
		{"missing user", "", "phone-1", sentAt, domain.ErrUserRequired},
		{"missing device", "staff-1", " ", sentAt, domain.ErrDeviceRequired},
		{"missing time", "staff-1", "phone-1", time.Time{}, domain.ErrStateTimeRequired},
	}

	for _, tt := range tests {
		if _, err := domain.NewConnectionState("tenant", "event", tt.user, tt.device, false, tt.at); !errors.Is(err, tt.want) {
			t.Errorf("%s: NewConnectionState error = %v, want %v", tt.name, err, tt.want)
		}
	}
}

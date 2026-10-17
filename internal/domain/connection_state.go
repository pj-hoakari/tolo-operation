package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrUserRequired      = errors.New("user ID is required")
	ErrDeviceRequired    = errors.New("device ID is required")
	ErrStateTimeRequired = errors.New("connection state time is required")
)

type ConnectionState struct {
	TenantPublicID string
	EventID        string
	UserID         string
	DeviceID       string
	Connected      bool
	At             time.Time
}

func NewConnectionState(tenantPublicID, eventID, userID, deviceID string, connected bool, at time.Time) (ConnectionState, error) {
	tenantPublicID, eventID, err := requireScope(tenantPublicID, eventID)
	if err != nil {
		return ConnectionState{}, err
	}

	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ConnectionState{}, ErrUserRequired
	}

	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return ConnectionState{}, ErrDeviceRequired
	}

	if at.IsZero() {
		return ConnectionState{}, ErrStateTimeRequired
	}

	return ConnectionState{
		TenantPublicID: tenantPublicID,
		EventID:        eventID,
		UserID:         userID,
		DeviceID:       deviceID,
		Connected:      connected,
		At:             at,
	}, nil
}

package domain

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidPublicID = errors.New("invalid public ID")
	ErrTenantRequired  = errors.New("tenant public ID is required")
	ErrEventRequired   = errors.New("event ID is required")
)

const (
	publicIDByteLength = 8
	publicIDLength     = publicIDByteLength * 2
)

func newPublicID() (string, error) {
	buffer := make([]byte, publicIDByteLength)

	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate public ID: %w", err)
	}

	return hex.EncodeToString(buffer), nil
}

func parsePublicID(value string) (string, error) {
	if len(value) != publicIDLength || value != strings.ToLower(value) {
		return "", ErrInvalidPublicID
	}

	if _, err := hex.DecodeString(value); err != nil {
		return "", ErrInvalidPublicID
	}

	return value, nil
}

func requireScope(tenantPublicID, eventID string) (string, string, error) {
	tenantPublicID = strings.TrimSpace(tenantPublicID)
	if tenantPublicID == "" {
		return "", "", ErrTenantRequired
	}

	eventID = strings.TrimSpace(eventID)
	if eventID == "" {
		return "", "", ErrEventRequired
	}

	return tenantPublicID, eventID, nil
}

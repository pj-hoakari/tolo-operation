package repository

import (
	"context"
	"errors"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

var ErrStaffMessageNotFound = errors.New("staff message not found")

type StaffMessageRepository interface {
	Create(ctx context.Context, message domain.StaffMessage) error
	Revoke(ctx context.Context, tenantPublicID, eventID string, id domain.MessageID, revokedAt time.Time) error
	ListActive(ctx context.Context, eventID string, audiences []domain.Audience) ([]domain.StaffMessage, error)
}

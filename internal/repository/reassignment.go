package repository

import (
	"context"
	"errors"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

var ErrReassignmentNotFound = errors.New("reassignment not found")

type ReassignmentRepository interface {
	Create(ctx context.Context, reassignment domain.Reassignment) error
	FindByID(ctx context.Context, tenantPublicID string, id domain.ReassignmentID) (domain.Reassignment, error)
	SaveStatus(ctx context.Context, reassignment domain.Reassignment) error
	ListCurrent(ctx context.Context, tenantPublicID, eventID string) ([]domain.Reassignment, error)
}

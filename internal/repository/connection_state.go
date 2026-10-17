package repository

import (
	"context"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

type ConnectionStateRepository interface {
	Save(ctx context.Context, state domain.ConnectionState) error
}

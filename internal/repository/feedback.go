package repository

import (
	"context"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

type FeedbackRepository interface {
	Record(ctx context.Context, values domain.FeedbackValues) error
}

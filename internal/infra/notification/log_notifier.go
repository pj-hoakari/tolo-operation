package notification

import (
	"context"
	"log/slog"

	"github.com/pj-hoakari/tolo-operation/internal/application"
)

type LogNotifier struct{}

func NewLogNotifier() *LogNotifier {
	return &LogNotifier{}
}

func (n *LogNotifier) SendPush(ctx context.Context, request application.PushRequest) ([]application.PushResult, error) {
	slog.InfoContext(ctx, "notification SendPush (log-only skeleton)",
		"event_id", request.EventID,
		"recipient_user_ids", request.RecipientUserIDs,
		"title", request.Message.Title,
		"data", request.Message.Data,
	)

	results := make([]application.PushResult, 0, len(request.RecipientUserIDs))
	for _, userID := range request.RecipientUserIDs {
		results = append(results, application.PushResult{UserID: userID, Sent: true, FailureReason: ""})
	}

	return results, nil
}

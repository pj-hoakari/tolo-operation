package connect

import (
	"context"
	"errors"
	"log/slog"

	connectrpc "connectrpc.com/connect/v2"

	"github.com/pj-hoakari/tolo-operation/internal/application"
	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
	"github.com/pj-hoakari/tolo-operation/internal/tenantctx"
)

// errInternal is the only detail a client learns about an internal failure.
var errInternal = errors.New("internal error")

var clientErrors = []struct {
	target  error
	code    connectrpc.Code
	message string
}{
	{domain.ErrInvalidPublicID, connectrpc.CodeInvalidArgument, "invalid public ID"},
	{domain.ErrTenantRequired, connectrpc.CodeInvalidArgument, "tenant_id is required"},
	{domain.ErrEventRequired, connectrpc.CodeInvalidArgument, "event_id is required"},
	{domain.ErrAudienceRequired, connectrpc.CodeInvalidArgument, "audience is required"},
	{domain.ErrBodyRequired, connectrpc.CodeInvalidArgument, "body is required"},
	{domain.ErrBodyTooLong, connectrpc.CodeInvalidArgument, "body must be at most 2000 characters"},
	{domain.ErrRouteIDRequired, connectrpc.CodeInvalidArgument, "related_route_ids must not contain an empty ID"},
	{domain.ErrStaffRequired, connectrpc.CodeInvalidArgument, "staff_user_id is required"},
	{domain.ErrTargetRequired, connectrpc.CodeInvalidArgument, "target is required"},
	{domain.ErrTargetIDRequired, connectrpc.CodeInvalidArgument, "target ID is required"},
	{domain.ErrInvalidRoutePosition, connectrpc.CodeInvalidArgument, "route_position must be from 0 to 1 on a route anchor"},
	{domain.ErrUserRequired, connectrpc.CodeInvalidArgument, "user_id is required"},
	{domain.ErrDeviceRequired, connectrpc.CodeInvalidArgument, "device_id is required"},
	{domain.ErrStateTimeRequired, connectrpc.CodeInvalidArgument, "at is required"},
	{domain.ErrComputedAtRequired, connectrpc.CodeInvalidArgument, "values.computed_at is required"},
	{domain.ErrFeedbackKindRequired, connectrpc.CodeInvalidArgument, "feedback kind is required"},
	{domain.ErrFeedbackValueNotFinite, connectrpc.CodeInvalidArgument, "feedback value must be finite"},
	{repository.ErrStaffMessageNotFound, connectrpc.CodeNotFound, "staff message not found"},
	{repository.ErrReassignmentNotFound, connectrpc.CodeNotFound, "reassignment not found"},
	{tenantctx.ErrSubjectMissing, connectrpc.CodePermissionDenied, "subject is missing from the token"},
	{tenantctx.ErrMismatch, connectrpc.CodePermissionDenied, "tenant ID does not match the token"},
	{tenantctx.ErrEventMissing, connectrpc.CodePermissionDenied, "event ID is missing from the token"},
	{tenantctx.ErrEventMismatch, connectrpc.CodePermissionDenied, "event ID does not match the token"},
	{application.ErrForeignUser, connectrpc.CodePermissionDenied, "user_id does not match the token"},
	{application.ErrGuestAudienceOnly, connectrpc.CodePermissionDenied, "service callers may list guest messages only"},
}

func connectError(ctx context.Context, err error) *connectrpc.Error {
	for _, e := range clientErrors {
		if errors.Is(err, e.target) {
			return connectrpc.NewError(e.code, e.message).WithCause(err)
		}
	}

	return InternalError(ctx, err)
}

// InternalError reports a failure the client can do nothing about. The cause is
// written to the server log and replaced by a fixed message, so that no
// internal detail leaves the service. The log handler names the trace of the
// request context on the record, so an operator can find the failure in the
// trace it belongs to.
//
// A cancelled or timed-out request is the client going away rather than a
// server fault, so it keeps its own code and is not logged.
//
// It is exported for the other transports of this process, so that every
// service answers an internal failure the same way.
func InternalError(ctx context.Context, err error) *connectrpc.Error {
	if errors.Is(err, context.Canceled) {
		return connectrpc.NewError(connectrpc.CodeCanceled, "canceled").WithCause(err)
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return connectrpc.NewError(connectrpc.CodeDeadlineExceeded, "deadline exceeded").WithCause(err)
	}

	slog.ErrorContext(ctx, "internal error", "error", err)

	return connectrpc.NewError(connectrpc.CodeInternal, "internal error").WithCause(errInternal) //nolint:forbidigo // the one place that builds internal errors
}

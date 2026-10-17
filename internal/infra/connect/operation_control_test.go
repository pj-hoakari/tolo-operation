package connect

import (
	"context"
	"testing"

	connectrpc "connectrpc.com/connect/v2"
	"google.golang.org/protobuf/proto"

	kernelv1 "github.com/pj-hoakari/tolo-kernel-proto/gen/tolo/kernel/v1"

	operationv1 "github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1"
	"github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1/operationv1connect"
	"github.com/pj-hoakari/tolo-operation/internal/application"
	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
	"github.com/pj-hoakari/tolo-operation/internal/tenantctx"
)

const testReassignmentID = "8877665544332211"

type fakeOperationControl struct {
	directInput  application.DirectReassignmentInput
	applyInput   application.ApplyReassignmentInput
	listInput    application.ListReassignmentsInput
	reassignment domain.Reassignment
	listed       []domain.Reassignment
	err          error
}

func (f *fakeOperationControl) DirectReassignment(_ context.Context, input application.DirectReassignmentInput) (domain.Reassignment, error) {
	f.directInput = input

	if f.err != nil {
		return domain.Reassignment{}, f.err
	}

	target, err := domain.NewReassignmentTarget(input.TargetKind, input.TargetID, input.RoutePosition)
	if err != nil {
		return domain.Reassignment{}, err
	}

	return domain.Reassignment{
		ID:          testReassignmentID,
		EventID:     input.EventID,
		StaffUserID: input.StaffUserID,
		Target:      target,
		Status:      domain.ReassignmentDirected,
	}, nil
}

func (f *fakeOperationControl) ApplyReassignment(_ context.Context, input application.ApplyReassignmentInput) (domain.Reassignment, error) {
	f.applyInput = input

	return f.reassignment, f.err
}

func (f *fakeOperationControl) ListReassignments(_ context.Context, input application.ListReassignmentsInput) ([]domain.Reassignment, error) {
	f.listInput = input

	return f.listed, f.err
}

func newOperationControlClient(t *testing.T, scope string, useCases *fakeOperationControl) (operationv1connect.OperationControlServiceClient, string) {
	t.Helper()

	authorization, keys := mintEventToken(t, scope)
	httpServer := newTestServer(t, newTestHandlerWith(t, keys, testUseCases{operationControl: useCases}))

	return operationv1connect.NewOperationControlServiceClient(newClient(httpServer)), authorization
}

func TestDirectReassignmentRoundTripsEachTarget(t *testing.T) {
	t.Parallel()

	position := 0.5

	tests := []struct {
		name string
		req  *operationv1.DirectReassignmentRequest
		want *operationv1.Reassignment
	}{
		{
			"observation point",
			&operationv1.DirectReassignmentRequest{Target: &operationv1.DirectReassignmentRequest_ObservationPointId{ObservationPointId: "op-1"}},
			&operationv1.Reassignment{Target: &operationv1.Reassignment_ObservationPointId{ObservationPointId: "op-1"}},
		},
		{
			"gate point",
			&operationv1.DirectReassignmentRequest{Target: &operationv1.DirectReassignmentRequest_GatePointId{GatePointId: "gate-1"}},
			&operationv1.Reassignment{Target: &operationv1.Reassignment_GatePointId{GatePointId: "gate-1"}},
		},
		{
			"graph point",
			&operationv1.DirectReassignmentRequest{Target: &operationv1.DirectReassignmentRequest_Anchor{Anchor: &kernelv1.GraphAnchor{
				Target: &kernelv1.GraphAnchor_PointId{PointId: "point-1"},
			}}},
			&operationv1.Reassignment{Target: &operationv1.Reassignment_Anchor{Anchor: &kernelv1.GraphAnchor{
				Target: &kernelv1.GraphAnchor_PointId{PointId: "point-1"},
			}}},
		},
		{
			"route with a position",
			&operationv1.DirectReassignmentRequest{Target: &operationv1.DirectReassignmentRequest_Anchor{Anchor: &kernelv1.GraphAnchor{
				Target:        &kernelv1.GraphAnchor_RouteId{RouteId: "route-1"},
				RoutePosition: &position,
			}}},
			&operationv1.Reassignment{Target: &operationv1.Reassignment_Anchor{Anchor: &kernelv1.GraphAnchor{
				Target:        &kernelv1.GraphAnchor_RouteId{RouteId: "route-1"},
				RoutePosition: &position,
			}}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			useCases := &fakeOperationControl{}
			client, authorization := newOperationControlClient(t, "events.operate", useCases)

			tt.req.TenantId, tt.req.EventId, tt.req.StaffUserId = testTenantPublicID, testEventPublicID, "staff-2"

			res, err := client.DirectReassignment(authorized(authorization), tt.req)
			if err != nil {
				t.Fatalf("DirectReassignment: %v", err)
			}

			if useCases.directInput.TenantID != testTenantPublicID || useCases.directInput.EventID != testEventPublicID {
				t.Errorf("input = %+v, want the request tenant and event", useCases.directInput)
			}

			tt.want.ReassignmentId, tt.want.EventId, tt.want.StaffUserId = testReassignmentID, testEventPublicID, "staff-2"
			tt.want.Status = operationv1.ReassignmentStatus_REASSIGNMENT_STATUS_DIRECTED

			if !proto.Equal(res, tt.want) {
				t.Errorf("DirectReassignment = %v, want %v", res, tt.want)
			}
		})
	}
}

func TestDirectReassignmentRejectsInvalidTargets(t *testing.T) {
	t.Parallel()

	position := 0.5

	for name, req := range map[string]*operationv1.DirectReassignmentRequest{
		"no target":            {},
		"anchor without point": {Target: &operationv1.DirectReassignmentRequest_Anchor{Anchor: &kernelv1.GraphAnchor{}}},
		"position on a point": {Target: &operationv1.DirectReassignmentRequest_Anchor{Anchor: &kernelv1.GraphAnchor{
			Target:        &kernelv1.GraphAnchor_PointId{PointId: "point-1"},
			RoutePosition: &position,
		}}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client, authorization := newOperationControlClient(t, "events.operate", &fakeOperationControl{})
			req.EventId, req.StaffUserId = testEventPublicID, "staff-2"

			_, err := client.DirectReassignment(authorized(authorization), req)
			if got, want := connectrpc.CodeOf(err), connectrpc.CodeInvalidArgument; got != want {
				t.Errorf("DirectReassignment error code = %v, want %v", got, want)
			}
		})
	}
}

func TestApplyReassignment(t *testing.T) {
	t.Parallel()

	target, err := domain.NewReassignmentTarget(domain.TargetGatePoint, "gate-1", nil)
	if err != nil {
		t.Fatalf("NewReassignmentTarget: %v", err)
	}

	useCases := &fakeOperationControl{reassignment: domain.Reassignment{
		ID:     testReassignmentID,
		Target: target,
		Status: domain.ReassignmentApplied,
	}}
	client, authorization := newOperationControlClient(t, "events.operate", useCases)

	res, err := client.ApplyReassignment(authorized(authorization), &operationv1.ApplyReassignmentRequest{
		ReassignmentId: testReassignmentID,
		EventId:        testEventPublicID,
		TenantId:       testTenantPublicID,
	})
	if err != nil {
		t.Fatalf("ApplyReassignment: %v", err)
	}

	wantInput := application.ApplyReassignmentInput{TenantID: testTenantPublicID, EventID: testEventPublicID, ReassignmentID: testReassignmentID}
	if useCases.applyInput != wantInput {
		t.Errorf("input = %+v, want %+v", useCases.applyInput, wantInput)
	}

	if res.GetStatus() != operationv1.ReassignmentStatus_REASSIGNMENT_STATUS_APPLIED || res.GetGatePointId() != "gate-1" {
		t.Errorf("ApplyReassignment = %v", res)
	}
}

func TestApplyReassignmentReportsAnUnknownReassignment(t *testing.T) {
	t.Parallel()

	client, authorization := newOperationControlClient(t, "events.operate", &fakeOperationControl{err: repository.ErrReassignmentNotFound})

	_, err := client.ApplyReassignment(authorized(authorization), &operationv1.ApplyReassignmentRequest{ReassignmentId: testReassignmentID})
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeNotFound; got != want {
		t.Errorf("ApplyReassignment error code = %v, want %v", got, want)
	}
}

func TestListReassignmentsNeedsOnlyTheReadScope(t *testing.T) {
	t.Parallel()

	target, err := domain.NewReassignmentTarget(domain.TargetObservationPoint, "op-1", nil)
	if err != nil {
		t.Fatalf("NewReassignmentTarget: %v", err)
	}

	useCases := &fakeOperationControl{listed: []domain.Reassignment{
		{ID: testReassignmentID, StaffUserID: "staff-2", Target: target, Status: domain.ReassignmentDirected},
	}}
	client, authorization := newOperationControlClient(t, "events.read", useCases)

	res, err := client.ListReassignments(authorized(authorization), &operationv1.ListReassignmentsRequest{
		TenantId: testTenantPublicID,
		EventId:  testEventPublicID,
	})
	if err != nil {
		t.Fatalf("ListReassignments: %v", err)
	}

	if want := (application.ListReassignmentsInput{TenantID: testTenantPublicID, EventID: testEventPublicID}); useCases.listInput != want {
		t.Errorf("input = %+v, want %+v", useCases.listInput, want)
	}

	if len(res.GetReassignments()) != 1 || res.GetReassignments()[0].GetObservationPointId() != "op-1" {
		t.Errorf("ListReassignments = %v", res)
	}

	_, err = client.DirectReassignment(authorized(authorization), &operationv1.DirectReassignmentRequest{EventId: testEventPublicID})
	if got, want := connectrpc.CodeOf(err), connectrpc.CodePermissionDenied; got != want {
		t.Errorf("DirectReassignment with events.read error code = %v, want %v", got, want)
	}
}

func TestOperationControlScopeErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		code connectrpc.Code
	}{
		{domain.ErrTenantRequired, connectrpc.CodeInvalidArgument},
		{domain.ErrEventRequired, connectrpc.CodeInvalidArgument},
		{tenantctx.ErrMismatch, connectrpc.CodePermissionDenied},
		{tenantctx.ErrEventMismatch, connectrpc.CodePermissionDenied},
	}

	for _, tt := range tests {
		client, authorization := newOperationControlClient(t, "events.operate events.read", &fakeOperationControl{err: tt.err})
		ctx := authorized(authorization)

		_, directErr := client.DirectReassignment(ctx, &operationv1.DirectReassignmentRequest{})
		_, applyErr := client.ApplyReassignment(ctx, &operationv1.ApplyReassignmentRequest{})
		_, listErr := client.ListReassignments(ctx, &operationv1.ListReassignmentsRequest{})

		for name, err := range map[string]error{"direct": directErr, "apply": applyErr, "list": listErr} {
			if got := connectrpc.CodeOf(err); got != tt.code {
				t.Errorf("%v: %s error code = %v, want %v", tt.err, name, got, tt.code)
			}
		}
	}
}

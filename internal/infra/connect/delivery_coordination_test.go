package connect

import (
	"context"
	"reflect"
	"testing"

	connectrpc "connectrpc.com/connect/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"
	"github.com/pj-hoakari/internal-jwt-handling/jwtgen"

	operationv1 "github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1"
	"github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1/operationv1connect"
	"github.com/pj-hoakari/tolo-operation/internal/application"
	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/tenantctx"
)

type fakeDeliveryCoordination struct {
	deliveryInput  application.RequestProposalDeliveryInput
	deliveryResult application.DeliveryResult
	stateInput     application.UpdateConnectionStateInput
	feedbackInput  application.RecordFeedbackValuesInput
	err            error
}

func (f *fakeDeliveryCoordination) RequestProposalDelivery(
	_ context.Context,
	input application.RequestProposalDeliveryInput,
) (application.DeliveryResult, error) {
	f.deliveryInput = input

	return f.deliveryResult, f.err
}

func (f *fakeDeliveryCoordination) UpdateConnectionState(_ context.Context, input application.UpdateConnectionStateInput) error {
	f.stateInput = input

	return f.err
}

func (f *fakeDeliveryCoordination) RecordFeedbackValues(_ context.Context, input application.RecordFeedbackValuesInput) error {
	f.feedbackInput = input

	return f.err
}

func newDeliveryCoordinationClient(
	t *testing.T,
	config jwtgen.Config,
	useCases *fakeDeliveryCoordination,
) (operationv1connect.DeliveryCoordinationServiceClient, string) {
	t.Helper()

	authorization, keys := mintInternalJWTFor(t, config)
	httpServer := newTestServer(t, newTestHandlerWith(t, keys, testUseCases{delivery: useCases}))

	return operationv1connect.NewDeliveryCoordinationServiceClient(newClient(httpServer)), authorization
}

var observationServiceToken = jwtgen.Config{TokenUse: internaljwt.TokenUseService, Subject: "tolo-observation"}

func TestUpdateConnectionState(t *testing.T) {
	t.Parallel()

	useCases := &fakeDeliveryCoordination{}
	client, authorization := newDeliveryCoordinationClient(t, staffToken("events.read"), useCases)

	_, err := client.UpdateConnectionState(authorized(authorization), &operationv1.UpdateConnectionStateRequest{
		TenantId:  testTenantPublicID,
		EventId:   testEventPublicID,
		UserId:    testSubject,
		DeviceId:  "phone-1",
		Connected: true,
		At:        timestamppb.New(testSentAt),
	})
	if err != nil {
		t.Fatalf("UpdateConnectionState: %v", err)
	}

	want := application.UpdateConnectionStateInput{
		TenantID:  testTenantPublicID,
		EventID:   testEventPublicID,
		UserID:    testSubject,
		DeviceID:  "phone-1",
		Connected: true,
		At:        testSentAt,
	}
	if useCases.stateInput != want {
		t.Errorf("input = %+v, want %+v", useCases.stateInput, want)
	}
}

func TestUpdateConnectionStateErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		code connectrpc.Code
	}{
		{domain.ErrStateTimeRequired, connectrpc.CodeInvalidArgument},
		{domain.ErrDeviceRequired, connectrpc.CodeInvalidArgument},
		{application.ErrForeignUser, connectrpc.CodePermissionDenied},
		{domain.ErrTenantRequired, connectrpc.CodeInvalidArgument},
		{domain.ErrEventRequired, connectrpc.CodeInvalidArgument},
		{tenantctx.ErrMismatch, connectrpc.CodePermissionDenied},
		{tenantctx.ErrEventMismatch, connectrpc.CodePermissionDenied},
	}

	for _, tt := range tests {
		client, authorization := newDeliveryCoordinationClient(t, staffToken("events.read"), &fakeDeliveryCoordination{err: tt.err})

		_, err := client.UpdateConnectionState(authorized(authorization), &operationv1.UpdateConnectionStateRequest{UserId: testSubject})
		if got := connectrpc.CodeOf(err); got != tt.code {
			t.Errorf("%v: UpdateConnectionState error code = %v, want %v", tt.err, got, tt.code)
		}
	}
}

func TestUpdateConnectionStateLeavesAMissingTimeUnset(t *testing.T) {
	t.Parallel()

	useCases := &fakeDeliveryCoordination{}
	client, authorization := newDeliveryCoordinationClient(t, staffToken("events.read"), useCases)

	if _, err := client.UpdateConnectionState(authorized(authorization), &operationv1.UpdateConnectionStateRequest{UserId: testSubject}); err != nil {
		t.Fatalf("UpdateConnectionState: %v", err)
	}

	if !useCases.stateInput.At.IsZero() {
		t.Errorf("at = %v, want the zero time the domain rejects", useCases.stateInput.At)
	}
}

func TestRecordFeedbackValues(t *testing.T) {
	t.Parallel()

	useCases := &fakeDeliveryCoordination{}
	client, authorization := newDeliveryCoordinationClient(t, observationServiceToken, useCases)

	_, err := client.RecordFeedbackValues(authorized(authorization), &operationv1.RecordFeedbackValuesRequest{
		TenantId: testTenantPublicID,
		EventId:  testEventPublicID,
		Values: &operationv1.FeedbackValues{
			ComputedAt: timestamppb.New(testSentAt),
			Entries: []*operationv1.FeedbackEntry{
				{AttributeTag: "", Kind: "objective", Value: 0.75},
				{AttributeTag: "family", Kind: "wait_minutes", Value: 4},
			},
		},
	})
	if err != nil {
		t.Fatalf("RecordFeedbackValues: %v", err)
	}

	want := application.RecordFeedbackValuesInput{
		TenantID:   testTenantPublicID,
		EventID:    testEventPublicID,
		ComputedAt: testSentAt,
		Entries: []domain.FeedbackEntry{
			{AttributeTag: "", Kind: "objective", Value: 0.75},
			{AttributeTag: "family", Kind: "wait_minutes", Value: 4},
		},
	}
	if !reflect.DeepEqual(useCases.feedbackInput, want) {
		t.Errorf("input = %+v, want %+v", useCases.feedbackInput, want)
	}
}

func TestRecordFeedbackValuesErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		code connectrpc.Code
	}{
		{domain.ErrTenantRequired, connectrpc.CodeInvalidArgument},
		{tenantctx.ErrMismatch, connectrpc.CodePermissionDenied},
	}

	for _, tt := range tests {
		client, authorization := newDeliveryCoordinationClient(t, observationServiceToken, &fakeDeliveryCoordination{err: tt.err})

		_, err := client.RecordFeedbackValues(authorized(authorization), &operationv1.RecordFeedbackValuesRequest{EventId: testEventPublicID})
		if got := connectrpc.CodeOf(err); got != tt.code {
			t.Errorf("%v: RecordFeedbackValues error code = %v, want %v", tt.err, got, tt.code)
		}
	}
}

func TestRecordFeedbackValuesRejectsAnEventToken(t *testing.T) {
	t.Parallel()

	client, authorization := newDeliveryCoordinationClient(t, staffToken("events.operate events.read"), &fakeDeliveryCoordination{})

	_, err := client.RecordFeedbackValues(authorized(authorization), &operationv1.RecordFeedbackValuesRequest{EventId: testEventPublicID})
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
		t.Errorf("RecordFeedbackValues error code = %v, want %v", got, want)
	}
}

func TestRequestProposalDeliveryPassesEitherPayloadAndAnswersTheCounts(t *testing.T) {
	t.Parallel()

	limit := 12.5
	flow := &operationv1.FlowProposals{
		RestrictionProposals: []*operationv1.RestrictionProposal{{
			RouteId:    "route-a",
			Action:     operationv1.RestrictionAction_RESTRICTION_ACTION_LIMIT,
			LimitValue: &limit,
		}},
	}
	line := &operationv1.LineGuidance{
		Availability: []*operationv1.GateAvailability{{GatePointId: "gate-1", Available: true}},
	}

	tests := []struct {
		name    string
		request *operationv1.RequestProposalDeliveryRequest
		want    *operationv1.DeliveryPayload
	}{
		{
			name: "flow proposals",
			request: &operationv1.RequestProposalDeliveryRequest{
				EventId:  testEventPublicID,
				TenantId: testTenantPublicID,
				Payload:  &operationv1.RequestProposalDeliveryRequest_FlowProposals{FlowProposals: flow},
			},
			want: &operationv1.DeliveryPayload{Kind: &operationv1.DeliveryPayload_FlowProposals{FlowProposals: flow}},
		},
		{
			name: "line guidance",
			request: &operationv1.RequestProposalDeliveryRequest{
				EventId:  testEventPublicID,
				TenantId: testTenantPublicID,
				Payload:  &operationv1.RequestProposalDeliveryRequest_LineGuidance{LineGuidance: line},
			},
			want: &operationv1.DeliveryPayload{Kind: &operationv1.DeliveryPayload_LineGuidance{LineGuidance: line}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			useCases := &fakeDeliveryCoordination{deliveryResult: application.DeliveryResult{
				DeliveryID: "0199f2a8-0000-7000-8000-000000000000", RealtimeCount: 2, NotificationCount: 1,
			}}
			client, authorization := newDeliveryCoordinationClient(t, observationServiceToken, useCases)

			res, err := client.RequestProposalDelivery(authorized(authorization), tt.request)
			if err != nil {
				t.Fatalf("RequestProposalDelivery: %v", err)
			}

			if res.GetDeliveryId() != "0199f2a8-0000-7000-8000-000000000000" || res.GetRealtimeCount() != 2 || res.GetNotificationCount() != 1 {
				t.Errorf("response = %v, want the delivery ID and counts of the use case", res)
			}

			input := useCases.deliveryInput
			if input.TenantID != testTenantPublicID || input.EventID != testEventPublicID || !proto.Equal(input.Proposal, tt.want) {
				t.Errorf("input = %+v, want proposal %v for the request pair", input, tt.want)
			}
		})
	}
}

func TestRequestProposalDeliveryRejectsAMissingPayload(t *testing.T) {
	t.Parallel()

	useCases := &fakeDeliveryCoordination{err: domain.ErrProposalRequired}
	client, authorization := newDeliveryCoordinationClient(t, observationServiceToken, useCases)

	_, err := client.RequestProposalDelivery(authorized(authorization), &operationv1.RequestProposalDeliveryRequest{
		EventId:  testEventPublicID,
		TenantId: testTenantPublicID,
	})
	if got := connectrpc.CodeOf(err); got != connectrpc.CodeInvalidArgument {
		t.Errorf("RequestProposalDelivery error code = %v, want %v", got, connectrpc.CodeInvalidArgument)
	}

	if useCases.deliveryInput.Proposal.GetKind() != nil {
		t.Errorf("input = %+v, want no proposal for a request without payload", useCases.deliveryInput)
	}
}

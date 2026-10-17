package connect

import (
	"context"
	"reflect"
	"testing"

	connectrpc "connectrpc.com/connect/v2"
	"google.golang.org/protobuf/types/known/timestamppb"

	internaljwt "github.com/pj-hoakari/internal-jwt-handling"
	"github.com/pj-hoakari/internal-jwt-handling/jwtgen"

	operationv1 "github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1"
	"github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1/operationv1connect"
	"github.com/pj-hoakari/tolo-operation/internal/application"
	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

type fakeDeliveryCoordination struct {
	stateInput    application.UpdateConnectionStateInput
	feedbackInput application.RecordFeedbackValuesInput
	err           error
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
		UserId:    testSubject,
		DeviceId:  "phone-1",
		Connected: true,
		At:        timestamppb.New(testSentAt),
	})
	if err != nil {
		t.Fatalf("UpdateConnectionState: %v", err)
	}

	want := application.UpdateConnectionStateInput{UserID: testSubject, DeviceID: "phone-1", Connected: true, At: testSentAt}
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
		EventId: testEventPublicID,
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

func TestRecordFeedbackValuesRejectsAnEventToken(t *testing.T) {
	t.Parallel()

	client, authorization := newDeliveryCoordinationClient(t, staffToken("events.operate events.read"), &fakeDeliveryCoordination{})

	_, err := client.RecordFeedbackValues(authorized(authorization), &operationv1.RecordFeedbackValuesRequest{EventId: testEventPublicID})
	if got, want := connectrpc.CodeOf(err), connectrpc.CodeUnauthenticated; got != want {
		t.Errorf("RecordFeedbackValues error code = %v, want %v", got, want)
	}
}

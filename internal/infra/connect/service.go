package connect

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	kernelv1 "github.com/pj-hoakari/tolo-kernel-proto/gen/tolo/kernel/v1"
	operationv1 "github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1"
	"github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1/operationv1connect"
	"github.com/pj-hoakari/tolo-operation/internal/application"
	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

type StaffCommunicationService struct {
	operationv1connect.UnimplementedStaffCommunicationServiceHandler

	useCases application.StaffCommunicationUseCases
}

func NewStaffCommunicationService(useCases application.StaffCommunicationUseCases) *StaffCommunicationService {
	return &StaffCommunicationService{
		UnimplementedStaffCommunicationServiceHandler: operationv1connect.UnimplementedStaffCommunicationServiceHandler{},
		useCases: useCases,
	}
}

func (s *StaffCommunicationService) SendStaffMessage(
	ctx context.Context,
	req *operationv1.SendStaffMessageRequest,
) (*operationv1.StaffMessage, error) {
	message, err := s.useCases.SendStaffMessage(ctx, application.SendStaffMessageInput{
		EventID:  req.GetEventId(),
		Audience: domainAudience(req.GetAudience()),
		Body:     req.GetBody(),
	})
	if err != nil {
		return nil, connectError(ctx, err)
	}

	return newProtoStaffMessage(message), nil
}

func (s *StaffCommunicationService) RevokeStaffMessage(
	ctx context.Context,
	req *operationv1.RevokeStaffMessageRequest,
) (*operationv1.RevokeStaffMessageResponse, error) {
	err := s.useCases.RevokeStaffMessage(ctx, application.RevokeStaffMessageInput{
		EventID:   req.GetEventId(),
		MessageID: req.GetMessageId(),
	})
	if err != nil {
		return nil, connectError(ctx, err)
	}

	return &operationv1.RevokeStaffMessageResponse{}, nil
}

func (s *StaffCommunicationService) ShareGuidanceInfo(
	ctx context.Context,
	req *operationv1.ShareGuidanceInfoRequest,
) (*operationv1.StaffMessage, error) {
	message, err := s.useCases.ShareGuidanceInfo(ctx, application.ShareGuidanceInfoInput{
		EventID:         req.GetEventId(),
		Body:            req.GetBody(),
		RelatedRouteIDs: req.GetRelatedRouteIds(),
	})
	if err != nil {
		return nil, connectError(ctx, err)
	}

	return newProtoStaffMessage(message), nil
}

func (s *StaffCommunicationService) ListStaffMessages(
	ctx context.Context,
	req *operationv1.ListStaffMessagesRequest,
) (*operationv1.ListStaffMessagesResponse, error) {
	messages, err := s.useCases.ListStaffMessages(ctx, application.ListStaffMessagesInput{
		EventID:  req.GetEventId(),
		Audience: domainAudience(req.GetAudience()),
	})
	if err != nil {
		return nil, connectError(ctx, err)
	}

	res := &operationv1.ListStaffMessagesResponse{Messages: make([]*operationv1.StaffMessage, 0, len(messages))}
	for _, message := range messages {
		res.Messages = append(res.Messages, newProtoStaffMessage(message))
	}

	return res, nil
}

func newProtoStaffMessage(message domain.StaffMessage) *operationv1.StaffMessage {
	return &operationv1.StaffMessage{
		MessageId:    string(message.ID),
		EventId:      message.EventID,
		SenderUserId: message.SenderUserID,
		Audience:     protoAudience(message.Audience),
		Body:         message.Body,
		SentAt:       timestamppb.New(message.SentAt),
	}
}

var audiences = map[operationv1.Audience]domain.Audience{
	operationv1.Audience_AUDIENCE_STAFF: domain.AudienceStaff,
	operationv1.Audience_AUDIENCE_GUEST: domain.AudienceGuest,
	operationv1.Audience_AUDIENCE_BOTH:  domain.AudienceBoth,
}

func domainAudience(audience operationv1.Audience) domain.Audience {
	return audiences[audience]
}

func protoAudience(audience domain.Audience) operationv1.Audience {
	for protoValue, domainValue := range audiences {
		if domainValue == audience {
			return protoValue
		}
	}

	return operationv1.Audience_AUDIENCE_UNSPECIFIED
}

type OperationControlService struct {
	operationv1connect.UnimplementedOperationControlServiceHandler

	useCases application.OperationControlUseCases
}

func NewOperationControlService(useCases application.OperationControlUseCases) *OperationControlService {
	return &OperationControlService{
		UnimplementedOperationControlServiceHandler: operationv1connect.UnimplementedOperationControlServiceHandler{},
		useCases: useCases,
	}
}

func (s *OperationControlService) DirectReassignment(
	ctx context.Context,
	req *operationv1.DirectReassignmentRequest,
) (*operationv1.Reassignment, error) {
	input := application.DirectReassignmentInput{
		EventID:       req.GetEventId(),
		StaffUserID:   req.GetStaffUserId(),
		TargetKind:    "",
		TargetID:      "",
		RoutePosition: nil,
	}

	switch target := req.GetTarget().(type) {
	case *operationv1.DirectReassignmentRequest_ObservationPointId:
		input.TargetKind, input.TargetID = domain.TargetObservationPoint, target.ObservationPointId
	case *operationv1.DirectReassignmentRequest_GatePointId:
		input.TargetKind, input.TargetID = domain.TargetGatePoint, target.GatePointId
	case *operationv1.DirectReassignmentRequest_Anchor:
		input.RoutePosition = target.Anchor.RoutePosition

		switch anchor := target.Anchor.GetTarget().(type) {
		case *kernelv1.GraphAnchor_PointId:
			input.TargetKind, input.TargetID = domain.TargetGraphPoint, anchor.PointId
		case *kernelv1.GraphAnchor_RouteId:
			input.TargetKind, input.TargetID = domain.TargetGraphRoute, anchor.RouteId
		}
	}

	reassignment, err := s.useCases.DirectReassignment(ctx, input)
	if err != nil {
		return nil, connectError(ctx, err)
	}

	return newProtoReassignment(reassignment), nil
}

func (s *OperationControlService) ApplyReassignment(
	ctx context.Context,
	req *operationv1.ApplyReassignmentRequest,
) (*operationv1.Reassignment, error) {
	reassignment, err := s.useCases.ApplyReassignment(ctx, application.ApplyReassignmentInput{
		ReassignmentID: req.GetReassignmentId(),
	})
	if err != nil {
		return nil, connectError(ctx, err)
	}

	return newProtoReassignment(reassignment), nil
}

func (s *OperationControlService) ListReassignments(
	ctx context.Context,
	req *operationv1.ListReassignmentsRequest,
) (*operationv1.ListReassignmentsResponse, error) {
	reassignments, err := s.useCases.ListReassignments(ctx, application.ListReassignmentsInput{EventID: req.GetEventId()})
	if err != nil {
		return nil, connectError(ctx, err)
	}

	res := &operationv1.ListReassignmentsResponse{Reassignments: make([]*operationv1.Reassignment, 0, len(reassignments))}
	for _, reassignment := range reassignments {
		res.Reassignments = append(res.Reassignments, newProtoReassignment(reassignment))
	}

	return res, nil
}

func newProtoReassignment(reassignment domain.Reassignment) *operationv1.Reassignment {
	res := &operationv1.Reassignment{
		ReassignmentId: string(reassignment.ID),
		EventId:        reassignment.EventID,
		StaffUserId:    reassignment.StaffUserID,
		Target:         nil,
		Status:         operationv1.ReassignmentStatus_REASSIGNMENT_STATUS_DIRECTED,
	}

	if reassignment.Status == domain.ReassignmentApplied {
		res.Status = operationv1.ReassignmentStatus_REASSIGNMENT_STATUS_APPLIED
	}

	target := reassignment.Target
	anchor := &kernelv1.GraphAnchor{Target: nil, RoutePosition: nil}

	if position, ok := target.RoutePosition(); ok {
		anchor.RoutePosition = &position
	}

	switch target.Kind() {
	case domain.TargetObservationPoint:
		res.Target = &operationv1.Reassignment_ObservationPointId{ObservationPointId: target.ID()}
	case domain.TargetGatePoint:
		res.Target = &operationv1.Reassignment_GatePointId{GatePointId: target.ID()}
	case domain.TargetGraphPoint:
		anchor.Target = &kernelv1.GraphAnchor_PointId{PointId: target.ID()}
		res.Target = &operationv1.Reassignment_Anchor{Anchor: anchor}
	case domain.TargetGraphRoute:
		anchor.Target = &kernelv1.GraphAnchor_RouteId{RouteId: target.ID()}
		res.Target = &operationv1.Reassignment_Anchor{Anchor: anchor}
	}

	return res
}

type DeliveryCoordinationService struct {
	operationv1connect.UnimplementedDeliveryCoordinationServiceHandler
}

func NewDeliveryCoordinationService() *DeliveryCoordinationService {
	return &DeliveryCoordinationService{
		UnimplementedDeliveryCoordinationServiceHandler: operationv1connect.UnimplementedDeliveryCoordinationServiceHandler{},
	}
}

type DigestService struct {
	operationv1connect.UnimplementedDigestServiceHandler
}

func NewDigestService() *DigestService {
	return &DigestService{
		UnimplementedDigestServiceHandler: operationv1connect.UnimplementedDigestServiceHandler{},
	}
}

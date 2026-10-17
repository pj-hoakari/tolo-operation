package connect

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

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
}

func NewOperationControlService() *OperationControlService {
	return &OperationControlService{
		UnimplementedOperationControlServiceHandler: operationv1connect.UnimplementedOperationControlServiceHandler{},
	}
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

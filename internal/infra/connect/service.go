package connect

import (
	"github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1/operationv1connect"
)

type StaffCommunicationService struct {
	operationv1connect.UnimplementedStaffCommunicationServiceHandler
}

func NewStaffCommunicationService() *StaffCommunicationService {
	return &StaffCommunicationService{
		UnimplementedStaffCommunicationServiceHandler: operationv1connect.UnimplementedStaffCommunicationServiceHandler{},
	}
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

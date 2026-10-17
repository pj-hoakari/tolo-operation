package connect

import (
	"slices"
	"testing"

	"github.com/pj-hoakari/protoc-gen-authz-go/authz"

	"github.com/pj-hoakari/tolo-operation/gen/tolo/operation/v1/operationv1connect"
)

func TestGeneratedPoliciesMatchTheSpecification(t *testing.T) {
	t.Parallel()

	eventAccess := func(scope string) authz.Policy {
		return authz.Policy{Level: authz.LevelAuthenticated, TokenUses: []string{"event_access"}, RequiredScopes: []string{scope}}
	}
	internal := authz.Policy{Level: authz.LevelInternal}

	want := map[string]authz.Policy{
		operationv1connect.StaffCommunicationServiceSendStaffMessageProcedure:   eventAccess("events.operate"),
		operationv1connect.StaffCommunicationServiceRevokeStaffMessageProcedure: eventAccess("events.operate"),
		operationv1connect.StaffCommunicationServiceShareGuidanceInfoProcedure:  eventAccess("events.operate"),
		operationv1connect.StaffCommunicationServiceListStaffMessagesProcedure: {
			Level:          authz.LevelAuthenticated,
			TokenUses:      []string{"event_access", "service"},
			RequiredScopes: []string{"events.read"},
		},
		operationv1connect.OperationControlServiceDirectReassignmentProcedure:          eventAccess("events.operate"),
		operationv1connect.OperationControlServiceApplyReassignmentProcedure:           eventAccess("events.operate"),
		operationv1connect.OperationControlServiceListReassignmentsProcedure:           eventAccess("events.read"),
		operationv1connect.DeliveryCoordinationServiceRequestProposalDeliveryProcedure: internal,
		operationv1connect.DeliveryCoordinationServiceRecordFeedbackValuesProcedure:    internal,
		operationv1connect.DeliveryCoordinationServiceUpdateConnectionStateProcedure:   eventAccess("events.read"),
		operationv1connect.DigestServiceGetHistoryDigestProcedure:                      eventAccess("events.read"),
	}

	got, err := authz.Merge(
		operationv1connect.StaffCommunicationServicePolicies,
		operationv1connect.OperationControlServicePolicies,
		operationv1connect.DeliveryCoordinationServicePolicies,
		operationv1connect.DigestServicePolicies,
	)
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	for procedure, wantPolicy := range want {
		policy, ok := got.Lookup(procedure)
		if !ok {
			t.Errorf("%s is not named by the generated policies", procedure)

			continue
		}

		if policy.Level != wantPolicy.Level {
			t.Errorf("%s level = %v, want %v", procedure, policy.Level, wantPolicy.Level)
		}

		if !slices.Equal(policy.TokenUses, wantPolicy.TokenUses) {
			t.Errorf("%s token uses = %v, want %v", procedure, policy.TokenUses, wantPolicy.TokenUses)
		}

		if !slices.Equal(policy.RequiredScopes, wantPolicy.RequiredScopes) {
			t.Errorf("%s required scopes = %v, want %v", procedure, policy.RequiredScopes, wantPolicy.RequiredScopes)
		}
	}

	for procedure := range got {
		if _, ok := want[procedure]; !ok {
			t.Errorf("%s is served without a declared policy", procedure)
		}
	}
}

package application_test

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/pj-hoakari/tolo-operation/internal/application"
	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
	"github.com/pj-hoakari/tolo-operation/internal/tenantctx"
)

const testReassignmentID = "8877665544332211"

func newOperationControl(t *testing.T) (*application.OperationControlService, *MockReassignmentRepository) {
	t.Helper()

	reassignments := NewMockReassignmentRepository(gomock.NewController(t))

	return application.NewOperationControlService(reassignments, clock), reassignments
}

func storedReassignment(t *testing.T, eventID string, status domain.ReassignmentStatus) domain.Reassignment {
	t.Helper()

	target, err := domain.NewReassignmentTarget(domain.TargetGatePoint, "gate-1", nil)
	if err != nil {
		t.Fatalf("NewReassignmentTarget: %v", err)
	}

	return domain.Reassignment{
		ID:             testReassignmentID,
		TenantPublicID: testTenantPublicID,
		EventID:        eventID,
		StaffUserID:    "staff-2",
		Target:         target,
		Status:         status,
		DirectedAt:     testNow,
	}
}

func TestDirectReassignment(t *testing.T) {
	t.Parallel()

	service, reassignments := newOperationControl(t)
	reassignments.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil)

	position := 0.4

	reassignment, err := service.DirectReassignment(withEventToken(testEventPublicID), application.DirectReassignmentInput{
		EventID:       testEventPublicID,
		StaffUserID:   "staff-2",
		TargetKind:    domain.TargetGraphRoute,
		TargetID:      "route-1",
		RoutePosition: &position,
	})
	if err != nil {
		t.Fatalf("DirectReassignment: %v", err)
	}

	gotPosition, ok := reassignment.Target.RoutePosition()
	if reassignment.Status != domain.ReassignmentDirected || reassignment.TenantPublicID != testTenantPublicID ||
		reassignment.Target.Kind() != domain.TargetGraphRoute || reassignment.Target.ID() != "route-1" ||
		!ok || gotPosition != position || !reassignment.DirectedAt.Equal(testNow) {
		t.Errorf("DirectReassignment = %+v", reassignment)
	}
}

func TestDirectReassignmentRequiresATarget(t *testing.T) {
	t.Parallel()

	service, _ := newOperationControl(t)

	_, err := service.DirectReassignment(withEventToken(testEventPublicID), application.DirectReassignmentInput{
		EventID:     testEventPublicID,
		StaffUserID: "staff-2",
	})
	if !errors.Is(err, domain.ErrTargetRequired) {
		t.Errorf("DirectReassignment error = %v, want %v", err, domain.ErrTargetRequired)
	}
}

func TestApplyReassignmentMovesADirectedReassignmentToApplied(t *testing.T) {
	t.Parallel()

	service, reassignments := newOperationControl(t)
	reassignments.EXPECT().FindByID(gomock.Any(), testTenantPublicID, domain.ReassignmentID(testReassignmentID)).
		Return(storedReassignment(t, testEventPublicID, domain.ReassignmentDirected), nil)
	reassignments.EXPECT().SaveStatus(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, saved domain.Reassignment) error {
		if saved.Status != domain.ReassignmentApplied {
			t.Errorf("saved status = %v, want %v", saved.Status, domain.ReassignmentApplied)
		}

		return nil
	})

	applied, err := service.ApplyReassignment(withEventToken(testEventPublicID), application.ApplyReassignmentInput{ReassignmentID: testReassignmentID})
	if err != nil {
		t.Fatalf("ApplyReassignment: %v", err)
	}

	if applied.Status != domain.ReassignmentApplied {
		t.Errorf("status = %v, want %v", applied.Status, domain.ReassignmentApplied)
	}
}

func TestApplyReassignmentIsIdempotent(t *testing.T) {
	t.Parallel()

	service, reassignments := newOperationControl(t)
	stored := storedReassignment(t, testEventPublicID, domain.ReassignmentApplied)
	reassignments.EXPECT().FindByID(gomock.Any(), gomock.Any(), gomock.Any()).Return(stored, nil)

	applied, err := service.ApplyReassignment(withEventToken(testEventPublicID), application.ApplyReassignmentInput{ReassignmentID: testReassignmentID})
	if err != nil {
		t.Fatalf("ApplyReassignment: %v", err)
	}

	if applied != stored {
		t.Errorf("ApplyReassignment = %+v, want the stored reassignment unchanged %+v", applied, stored)
	}
}

func TestApplyReassignmentRejectsAnotherEventsReassignment(t *testing.T) {
	t.Parallel()

	service, reassignments := newOperationControl(t)
	reassignments.EXPECT().FindByID(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(storedReassignment(t, otherEventPublicID, domain.ReassignmentDirected), nil)

	_, err := service.ApplyReassignment(withEventToken(testEventPublicID), application.ApplyReassignmentInput{ReassignmentID: testReassignmentID})
	if !errors.Is(err, tenantctx.ErrEventMismatch) {
		t.Errorf("ApplyReassignment error = %v, want %v", err, tenantctx.ErrEventMismatch)
	}
}

func TestApplyReassignmentReportsAnUnknownReassignment(t *testing.T) {
	t.Parallel()

	service, reassignments := newOperationControl(t)
	reassignments.EXPECT().FindByID(gomock.Any(), gomock.Any(), gomock.Any()).Return(domain.Reassignment{}, repository.ErrReassignmentNotFound)

	_, err := service.ApplyReassignment(withEventToken(testEventPublicID), application.ApplyReassignmentInput{ReassignmentID: testReassignmentID})
	if !errors.Is(err, repository.ErrReassignmentNotFound) {
		t.Errorf("ApplyReassignment error = %v, want %v", err, repository.ErrReassignmentNotFound)
	}
}

func TestListReassignments(t *testing.T) {
	t.Parallel()

	service, reassignments := newOperationControl(t)
	current := []domain.Reassignment{storedReassignment(t, testEventPublicID, domain.ReassignmentApplied)}
	reassignments.EXPECT().ListCurrent(gomock.Any(), testTenantPublicID, testEventPublicID).Return(current, nil)

	got, err := service.ListReassignments(withEventToken(testEventPublicID), application.ListReassignmentsInput{EventID: testEventPublicID})
	if err != nil {
		t.Fatalf("ListReassignments: %v", err)
	}

	if len(got) != 1 || got[0] != current[0] {
		t.Errorf("ListReassignments = %+v, want %+v", got, current)
	}

	if _, err := service.ListReassignments(withEventToken(testEventPublicID), application.ListReassignmentsInput{EventID: otherEventPublicID}); !errors.Is(err, tenantctx.ErrEventMismatch) {
		t.Errorf("ListReassignments(other event) error = %v, want %v", err, tenantctx.ErrEventMismatch)
	}
}

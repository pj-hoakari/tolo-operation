package domain_test

import (
	"errors"
	"math"
	"testing"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
)

func ratio(value float64) *float64 {
	return &value
}

func TestNewReassignmentTarget(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		kind     domain.TargetKind
		id       string
		position *float64
		want     error
	}{
		{"observation point", domain.TargetObservationPoint, "op-1", nil, nil},
		{"gate point", domain.TargetGatePoint, "gate-1", nil, nil},
		{"graph point", domain.TargetGraphPoint, "point-1", nil, nil},
		{"graph route", domain.TargetGraphRoute, "route-1", nil, nil},
		{"graph route with a position", domain.TargetGraphRoute, "route-1", ratio(0.5), nil},
		{"route position at the ends", domain.TargetGraphRoute, "route-1", ratio(1), nil},
		{"no target", "", "op-1", nil, domain.ErrTargetRequired},
		{"unknown kind", "zone", "op-1", nil, domain.ErrTargetRequired},
		{"blank ID", domain.TargetGatePoint, " ", nil, domain.ErrTargetIDRequired},
		{"position on a point", domain.TargetGraphPoint, "point-1", ratio(0.5), domain.ErrInvalidRoutePosition},
		{"position on an observation point", domain.TargetObservationPoint, "op-1", ratio(0), domain.ErrInvalidRoutePosition},
		{"position past the route end", domain.TargetGraphRoute, "route-1", ratio(1.5), domain.ErrInvalidRoutePosition},
		{"negative position", domain.TargetGraphRoute, "route-1", ratio(-0.1), domain.ErrInvalidRoutePosition},
		{"NaN position", domain.TargetGraphRoute, "route-1", ratio(math.NaN()), domain.ErrInvalidRoutePosition},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			target, err := domain.NewReassignmentTarget(tt.kind, tt.id, tt.position)
			if !errors.Is(err, tt.want) {
				t.Fatalf("NewReassignmentTarget error = %v, want %v", err, tt.want)
			}

			if err != nil {
				return
			}

			position, ok := target.RoutePosition()
			if target.Kind() != tt.kind || target.ID() != tt.id || ok != (tt.position != nil) || (ok && position != *tt.position) {
				t.Errorf("target = %v %q %v %v, want %v %q %v", target.Kind(), target.ID(), position, ok, tt.kind, tt.id, tt.position)
			}
		})
	}
}

func TestDirectReassignment(t *testing.T) {
	t.Parallel()

	target, err := domain.NewReassignmentTarget(domain.TargetGatePoint, "gate-1", nil)
	if err != nil {
		t.Fatalf("NewReassignmentTarget: %v", err)
	}

	reassignment, err := domain.DirectReassignment("tenant", "event", " staff-2 ", target, sentAt)
	if err != nil {
		t.Fatalf("DirectReassignment: %v", err)
	}

	if !publicIDPattern.MatchString(string(reassignment.ID)) {
		t.Errorf("reassignment ID %q is not 16 lowercase hex characters", reassignment.ID)
	}

	if reassignment.Status != domain.ReassignmentDirected || reassignment.StaffUserID != "staff-2" ||
		reassignment.Target != target || !reassignment.DirectedAt.Equal(sentAt) {
		t.Errorf("DirectReassignment = %+v", reassignment)
	}
}

func TestDirectReassignmentRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	target, err := domain.NewReassignmentTarget(domain.TargetGatePoint, "gate-1", nil)
	if err != nil {
		t.Fatalf("NewReassignmentTarget: %v", err)
	}

	tests := []struct {
		name   string
		staff  string
		target domain.ReassignmentTarget
		want   error
	}{
		{"missing staff", " ", target, domain.ErrStaffRequired},
		{"missing target", "staff-2", domain.ReassignmentTarget{}, domain.ErrTargetRequired},
	}

	for _, tt := range tests {
		if _, err := domain.DirectReassignment("tenant", "event", tt.staff, tt.target, sentAt); !errors.Is(err, tt.want) {
			t.Errorf("%s: DirectReassignment error = %v, want %v", tt.name, err, tt.want)
		}
	}
}

func TestApplyReassignment(t *testing.T) {
	t.Parallel()

	target, err := domain.NewReassignmentTarget(domain.TargetObservationPoint, "op-1", nil)
	if err != nil {
		t.Fatalf("NewReassignmentTarget: %v", err)
	}

	directed, err := domain.DirectReassignment("tenant", "event", "staff-2", target, sentAt)
	if err != nil {
		t.Fatalf("DirectReassignment: %v", err)
	}

	applied, changed := directed.Apply()
	if !changed || applied.Status != domain.ReassignmentApplied {
		t.Fatalf("Apply() = %v, %v, want applied and changed", applied.Status, changed)
	}

	again, changed := applied.Apply()
	if changed || again != applied {
		t.Errorf("Apply() on an applied reassignment = %+v, %v, want it unchanged", again, changed)
	}
}

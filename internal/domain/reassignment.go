package domain

import (
	"errors"
	"math"
	"strings"
	"time"
)

var (
	ErrStaffRequired        = errors.New("staff user ID is required")
	ErrTargetRequired       = errors.New("reassignment target is required")
	ErrTargetIDRequired     = errors.New("reassignment target ID is required")
	ErrInvalidRoutePosition = errors.New("route position must be a ratio from 0 to 1 on a route")
)

type ReassignmentID string

func NewReassignmentID() (ReassignmentID, error) {
	id, err := newPublicID()
	if err != nil {
		return "", err
	}

	return ReassignmentID(id), nil
}

func ParseReassignmentID(value string) (ReassignmentID, error) {
	id, err := parsePublicID(value)
	if err != nil {
		return "", err
	}

	return ReassignmentID(id), nil
}

type TargetKind string

const (
	TargetObservationPoint TargetKind = "observation_point"
	TargetGatePoint        TargetKind = "gate_point"
	TargetGraphPoint       TargetKind = "graph_point"
	TargetGraphRoute       TargetKind = "graph_route"
)

type ReassignmentTarget struct {
	kind          TargetKind
	id            string
	routePosition *float64
}

func NewReassignmentTarget(kind TargetKind, id string, routePosition *float64) (ReassignmentTarget, error) {
	switch kind {
	case TargetObservationPoint, TargetGatePoint, TargetGraphPoint, TargetGraphRoute:
	default:
		return ReassignmentTarget{}, ErrTargetRequired
	}

	id = strings.TrimSpace(id)
	if id == "" {
		return ReassignmentTarget{}, ErrTargetIDRequired
	}

	if routePosition != nil {
		position := *routePosition
		if kind != TargetGraphRoute || math.IsNaN(position) || position < 0 || position > 1 {
			return ReassignmentTarget{}, ErrInvalidRoutePosition
		}

		routePosition = &position
	}

	return ReassignmentTarget{kind: kind, id: id, routePosition: routePosition}, nil
}

func (t ReassignmentTarget) Kind() TargetKind {
	return t.kind
}

func (t ReassignmentTarget) ID() string {
	return t.id
}

func (t ReassignmentTarget) RoutePosition() (float64, bool) {
	if t.routePosition == nil {
		return 0, false
	}

	return *t.routePosition, true
}

type ReassignmentStatus string

const (
	ReassignmentDirected ReassignmentStatus = "directed"
	ReassignmentApplied  ReassignmentStatus = "applied"
)

type Reassignment struct {
	ID             ReassignmentID
	TenantPublicID string
	EventID        string
	StaffUserID    string
	Target         ReassignmentTarget
	Status         ReassignmentStatus
	DirectedAt     time.Time
}

func DirectReassignment(
	tenantPublicID, eventID, staffUserID string,
	target ReassignmentTarget,
	directedAt time.Time,
) (Reassignment, error) {
	tenantPublicID, eventID, err := requireScope(tenantPublicID, eventID)
	if err != nil {
		return Reassignment{}, err
	}

	staffUserID = strings.TrimSpace(staffUserID)
	if staffUserID == "" {
		return Reassignment{}, ErrStaffRequired
	}

	if target.kind == "" {
		return Reassignment{}, ErrTargetRequired
	}

	id, err := NewReassignmentID()
	if err != nil {
		return Reassignment{}, err
	}

	return Reassignment{
		ID:             id,
		TenantPublicID: tenantPublicID,
		EventID:        eventID,
		StaffUserID:    staffUserID,
		Target:         target,
		Status:         ReassignmentDirected,
		DirectedAt:     directedAt,
	}, nil
}

func (r Reassignment) Apply() (Reassignment, bool) {
	if r.Status == ReassignmentApplied {
		return r, false
	}

	r.Status = ReassignmentApplied

	return r, true
}

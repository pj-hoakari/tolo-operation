package db

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/pj-hoakari/tolo-operation/internal/domain"
	"github.com/pj-hoakari/tolo-operation/internal/repository"
)

func directReassignment(
	t *testing.T,
	repo *PostgresReassignmentRepository,
	tenantPublicID, eventID, staffUserID string,
	target domain.ReassignmentTarget,
	directedAt time.Time,
) domain.Reassignment {
	t.Helper()

	reassignment, err := domain.DirectReassignment(tenantPublicID, eventID, staffUserID, target, directedAt)
	if err != nil {
		t.Fatalf("DirectReassignment: %v", err)
	}

	if err := repo.Create(context.Background(), reassignment); err != nil {
		t.Fatalf("Create: %v", err)
	}

	return reassignment
}

func newTarget(t *testing.T, kind domain.TargetKind, id string, position *float64) domain.ReassignmentTarget {
	t.Helper()

	target, err := domain.NewReassignmentTarget(kind, id, position)
	if err != nil {
		t.Fatalf("NewReassignmentTarget: %v", err)
	}

	return target
}

func TestPostgresReassignmentRepositoryRoundTripsEveryTarget(t *testing.T) {
	repo := NewPostgresReassignmentRepository(testDB)
	tenant, event := "tenant-reassignment-targets", "event-reassignment-targets"
	position := 0.25

	for _, target := range []domain.ReassignmentTarget{
		newTarget(t, domain.TargetObservationPoint, "op-1", nil),
		newTarget(t, domain.TargetGatePoint, "gate-1", nil),
		newTarget(t, domain.TargetGraphPoint, "point-1", nil),
		newTarget(t, domain.TargetGraphRoute, "route-1", nil),
		newTarget(t, domain.TargetGraphRoute, "route-2", &position),
	} {
		want := directReassignment(t, repo, tenant, event, "staff-"+target.ID(), target, baseTime)

		got, err := repo.FindByID(context.Background(), tenant, want.ID)
		if err != nil {
			t.Fatalf("FindByID: %v", err)
		}

		got.DirectedAt = got.DirectedAt.UTC()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("FindByID = %+v, want %+v", got, want)
		}
	}
}

func TestPostgresReassignmentRepositoryFindIsScopedToTenant(t *testing.T) {
	repo := NewPostgresReassignmentRepository(testDB)
	reassignment := directReassignment(t, repo, "tenant-reassignment-owner", "event-reassignment-owner", "staff-1",
		newTarget(t, domain.TargetGatePoint, "gate-1", nil), baseTime)

	_, err := repo.FindByID(context.Background(), "tenant-reassignment-intruder", reassignment.ID)
	if !errors.Is(err, repository.ErrReassignmentNotFound) {
		t.Errorf("FindByID error = %v, want %v", err, repository.ErrReassignmentNotFound)
	}

	applied, _ := reassignment.Apply()
	applied.TenantPublicID = "tenant-reassignment-intruder"

	if err := repo.SaveStatus(context.Background(), applied); !errors.Is(err, repository.ErrReassignmentNotFound) {
		t.Errorf("SaveStatus error = %v, want %v", err, repository.ErrReassignmentNotFound)
	}
}

func TestPostgresReassignmentRepositorySaveStatus(t *testing.T) {
	repo := NewPostgresReassignmentRepository(testDB)
	tenant := "tenant-reassignment-apply"
	reassignment := directReassignment(t, repo, tenant, "event-reassignment-apply", "staff-1",
		newTarget(t, domain.TargetGatePoint, "gate-1", nil), baseTime)

	applied, _ := reassignment.Apply()
	if err := repo.SaveStatus(context.Background(), applied); err != nil {
		t.Fatalf("SaveStatus: %v", err)
	}

	got, err := repo.FindByID(context.Background(), tenant, reassignment.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}

	if got.Status != domain.ReassignmentApplied {
		t.Errorf("status = %v, want %v", got.Status, domain.ReassignmentApplied)
	}
}

func TestPostgresReassignmentRepositoryListsTheLatestPlacementPerStaff(t *testing.T) {
	repo := NewPostgresReassignmentRepository(testDB)
	tenant, event := "tenant-reassignment-current", "event-reassignment-current"
	gate := newTarget(t, domain.TargetGatePoint, "gate-1", nil)
	point := newTarget(t, domain.TargetObservationPoint, "op-1", nil)

	directReassignment(t, repo, tenant, event, "staff-a", gate, baseTime)
	latestA := directReassignment(t, repo, tenant, event, "staff-a", point, baseTime.Add(time.Minute))
	directReassignment(t, repo, tenant, event, "staff-a", gate, baseTime.Add(-time.Minute))
	latestB := directReassignment(t, repo, tenant, event, "staff-b", gate, baseTime)
	sameInstantFirst := directReassignment(t, repo, tenant, event, "staff-c", gate, baseTime)
	sameInstantLast := directReassignment(t, repo, tenant, event, "staff-c", point, baseTime)
	directReassignment(t, repo, tenant, "event-reassignment-current-other", "staff-d", gate, baseTime)
	directReassignment(t, repo, "tenant-reassignment-current-other", event, "staff-e", gate, baseTime)

	got, err := repo.ListCurrent(context.Background(), tenant, event)
	if err != nil {
		t.Fatalf("ListCurrent: %v", err)
	}

	ids := make([]domain.ReassignmentID, 0, len(got))
	for _, reassignment := range got {
		ids = append(ids, reassignment.ID)
	}

	want := []domain.ReassignmentID{latestA.ID, latestB.ID, sameInstantLast.ID}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("ListCurrent = %v, want %v (not %v)", ids, want, sameInstantFirst.ID)
	}
}

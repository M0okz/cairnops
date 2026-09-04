package incidents

import (
	"context"
	"errors"
	"testing"
)

type serviceStore struct {
	plan            AcknowledgementPlan
	completeResults []AcknowledgementResult
	openedDays      int
}

func (*serviceStore) List(context.Context, string, int) ([]Incident, error) { return nil, nil }
func (*serviceStore) ListForTarget(context.Context, string, string, int) ([]Incident, error) {
	return nil, nil
}
func (*serviceStore) Get(context.Context, string) (Incident, error) { return Incident{}, nil }
func (store *serviceStore) OpenedByDay(_ context.Context, days int) ([]OpenedDay, error) {
	store.openedDays = days
	return nil, nil
}
func (store *serviceStore) AcknowledgeLocal(context.Context, string, string, string) (AcknowledgementPlan, error) {
	return store.plan, nil
}
func (store *serviceStore) CompleteAcknowledgement(_ context.Context, _ string, results []AcknowledgementResult) (Incident, error) {
	store.completeResults = results
	incident := Incident{AcknowledgementSyncStatus: "synchronized"}
	for _, result := range results {
		if result.Error != "" {
			incident.AcknowledgementSyncStatus = "failed"
			incident.AcknowledgementSyncError = result.Error
		}
	}
	return incident, nil
}
func (*serviceStore) InvalidateEvidence(context.Context, string, string, string, string, string) (Incident, error) {
	return Incident{}, nil
}

func TestEvidenceInvalidationRequiresAMeaningfulReason(t *testing.T) {
	t.Parallel()
	service := NewService(&serviceStore{}, nil)
	if _, err := service.InvalidateEvidence(context.Background(), "incident", "evidence", "actor", "Gregory", "panne"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
}

func TestIncidentHistoryWindowStaysWithinThePublishedBounds(t *testing.T) {
	t.Parallel()
	store := &serviceStore{}
	service := NewService(store, nil)

	for _, days := range []int{0, 91} {
		if _, err := service.OpenedByDay(context.Background(), days); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected %d days to be rejected, got %v", days, err)
		}
	}
	if store.openedDays != 0 {
		t.Fatalf("invalid windows must not reach the store, got %d", store.openedDays)
	}

	if _, err := service.OpenedByDay(context.Background(), 12); err != nil {
		t.Fatal(err)
	}
	if store.openedDays != 12 {
		t.Fatalf("expected the valid window to reach the store, got %d", store.openedDays)
	}
}

type failingAcknowledger struct{}

func (failingAcknowledger) Acknowledge(context.Context, AcknowledgementTarget, string) error {
	return errors.New("permission refusée")
}

func TestAcknowledgementRemainsLocalWhenExternalSyncFails(t *testing.T) {
	t.Parallel()
	store := &serviceStore{plan: AcknowledgementPlan{
		Incident: Incident{ID: "incident", AcknowledgementSyncStatus: "pending"},
		Targets: []AcknowledgementTarget{{
			EvidenceID: "evidence", Origin: "zabbix",
			ConnectorID: "connector", ExternalEventID: "42",
		}},
	}}
	service := NewService(store, failingAcknowledger{})

	incident, err := service.Acknowledge(context.Background(), "incident", "actor", "Gregory")
	if err != nil {
		t.Fatal(err)
	}
	if incident.AcknowledgementSyncStatus != "failed" || len(store.completeResults) != 1 {
		t.Fatalf("unexpected acknowledgement result: %#v", incident)
	}
	if result := store.completeResults[0]; result.EvidenceID != "evidence" || result.Error != "permission refusée" {
		t.Fatalf("unexpected synchronization result: %#v", result)
	}
}

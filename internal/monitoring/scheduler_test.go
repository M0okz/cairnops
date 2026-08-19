package monitoring

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
)

type fakeStore struct {
	sources   []domain.Source
	completed []domain.Observation
	mutex     sync.Mutex
}

func (store *fakeStore) ClaimDue(_ context.Context, _ string, _ int, _ time.Duration) ([]domain.Source, error) {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	sources := store.sources
	store.sources = nil
	return sources, nil
}

func (store *fakeStore) Complete(_ context.Context, _ string, _ domain.Source, observation domain.Observation) error {
	store.mutex.Lock()
	defer store.mutex.Unlock()
	store.completed = append(store.completed, observation)
	return nil
}

type healthyChecker struct{}

func (healthyChecker) Check(_ context.Context, source domain.Source) domain.Observation {
	return domain.Observation{
		SourceID: source.ID, TargetID: source.TargetID,
		ObservedAt: time.Now().UTC(), Outcome: domain.OutcomeHealthy,
	}
}

func TestSchedulerExecutesEveryClaimedSource(t *testing.T) {
	t.Parallel()

	store := &fakeStore{sources: []domain.Source{
		schedulerSource("one"), schedulerSource("two"), schedulerSource("three"),
	}}
	scheduler := NewScheduler(store, healthyChecker{}, "worker", nil)
	if err := scheduler.tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.completed) != 3 {
		t.Fatalf("expected 3 observations, got %d", len(store.completed))
	}
}

func schedulerSource(id string) domain.Source {
	return domain.Source{
		ID: id, TargetID: "target", Name: id, Kind: domain.SourceHTTP,
		Interval: time.Minute, Timeout: time.Second, Config: json.RawMessage(`{}`),
	}
}

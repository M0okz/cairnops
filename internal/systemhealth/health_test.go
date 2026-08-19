package systemhealth

import (
	"testing"
	"time"
)

func TestWorkerComponentStatus(t *testing.T) {
	t.Parallel()

	lastSeen := time.Now().UTC()
	for name, test := range map[string]struct {
		instances int
		lastSeen  *time.Time
		want      ComponentStatus
	}{
		"operational": {instances: 2, lastSeen: &lastSeen, want: StatusOperational},
		"stale":       {lastSeen: &lastSeen, want: StatusStale},
		"unavailable": {want: StatusUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			component := workerComponent(test.instances, test.lastSeen)
			if component.Status != test.want {
				t.Fatalf("expected %q, got %q", test.want, component.Status)
			}
		})
	}
}

// La fenêtre de latence garde les dernières mesures, jamais davantage, et son
// maximum est celui de ce qu'elle garde encore — une pointe sortie de la
// fenêtre ne hante pas la page indéfiniment.
func TestDatabaseLatencyWindow(t *testing.T) {
	t.Parallel()

	store := &Store{}
	store.record(120)
	for index := 0; index < latencySamples; index++ {
		store.record(float64(index + 1))
	}
	database := store.record(7)

	if len(database.Samples) != latencySamples {
		t.Fatalf("expected the window to hold %d samples, got %d", latencySamples, len(database.Samples))
	}
	if database.LatencyMilliseconds != 7 {
		t.Fatalf("expected the last reading, got %v", database.LatencyMilliseconds)
	}
	if database.MaximumLatencyMilliseconds != float64(latencySamples) {
		t.Fatalf("the 120 ms spike left the window: expected %d, got %v", latencySamples, database.MaximumLatencyMilliseconds)
	}
	if database.MeasuredSince.IsZero() {
		t.Fatal("the window should date its first measurement")
	}
}

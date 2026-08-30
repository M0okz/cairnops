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

func TestPushFreshnessIsLongerThanWorkerHeartbeat(t *testing.T) {
	t.Parallel()
	if pushFreshness <= workerFreshness {
		t.Fatal("the external relay probe must tolerate more delay than an internal worker heartbeat")
	}
}

func TestPushComponentStatus(t *testing.T) {
	t.Parallel()
	checkedAt := time.Now().UTC()
	fresh := checkedAt.Add(-pushFreshness / 2)
	stale := checkedAt.Add(-pushFreshness * 2)
	for name, test := range map[string]struct {
		configured bool
		status     string
		lastSeen   *time.Time
		want       ComponentStatus
	}{
		"operational":  {configured: true, status: "operational", lastSeen: &fresh, want: StatusOperational},
		"stale":        {configured: true, status: "operational", lastSeen: &stale, want: StatusStale},
		"failed":       {configured: true, status: "unavailable", lastSeen: &fresh, want: StatusUnavailable},
		"unconfigured": {want: StatusUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			component := summarizePush(test.configured, test.status, test.lastSeen, "", checkedAt)
			if component.Status != test.want {
				t.Fatalf("expected %q, got %q", test.want, component.Status)
			}
		})
	}
}

func TestOIDCComponentDistinguishesGraceAndSuspension(t *testing.T) {
	t.Parallel()
	lastSeen := time.Now().UTC()
	if got := summarizeOIDC(2, 0, 1, &lastSeen); got.Status != StatusStale {
		t.Fatalf("a transient failure inside the grace period should be stale, got %q", got.Status)
	}
	if got := summarizeOIDC(2, 1, 1, &lastSeen); got.Status != StatusUnavailable {
		t.Fatalf("a suspended external access should be unavailable, got %q", got.Status)
	}
	if got := summarizeOIDC(0, 0, 0, nil); got.Status != StatusOperational {
		t.Fatalf("a tested configuration without Users should be operational, got %q", got.Status)
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

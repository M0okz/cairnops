package systemhealth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/M0okz/cairnops/internal/metrics"
	"github.com/jackc/pgx/v5/pgxpool"
)

const workerFreshness = 45 * time.Second

// Profondeur de la série horaire rendue avec la Santé : la même fenêtre que
// les mesures, pour qu'un chiffre et son micro-graphe parlent des mêmes heures.
const instanceHours = 24

// Nombre de mesures de latence gardées en mémoire. Elles ne sont pas écrites :
// la latence de PostgreSQL dit comment l'instance se porte maintenant, pas ce
// qu'elle a vécu — un redémarrage a le droit de l'oublier.
const latencySamples = 24

type ComponentStatus string

const (
	StatusOperational ComponentStatus = "operational"
	StatusStale       ComponentStatus = "stale"
	StatusUnavailable ComponentStatus = "unavailable"
)

type Component struct {
	Name       string          `json:"name"`
	Status     ComponentStatus `json:"status"`
	Instances  int             `json:"instances"`
	LastSeenAt *time.Time      `json:"last_seen_at,omitempty"`
}

// Database porte ce que le serveur sait du temps de réponse de PostgreSQL :
// la lecture qui vient d'avoir lieu, et le pire des lectures gardées. Aucun
// percentile : rien n'est stocké, et un p95 sur vingt-quatre mesures ne serait
// qu'un maximum déguisé.
type Database struct {
	LatencyMilliseconds        float64   `json:"latency_milliseconds"`
	MaximumLatencyMilliseconds float64   `json:"maximum_latency_milliseconds"`
	Samples                    []float64 `json:"samples"`
	MeasuredSince              time.Time `json:"measured_since"`
}

type Snapshot struct {
	Status     string         `json:"status"`
	CheckedAt  time.Time      `json:"checked_at"`
	Components []Component    `json:"components"`
	Database   Database       `json:"database"`
	Hours      []metrics.Hour `json:"hours"`
}

type Store struct {
	pool    *pgxpool.Pool
	metrics *metrics.Store

	// Les mesures de latence vivent en mémoire, partagées par toutes les
	// lectures concurrentes de la Santé.
	mutex   sync.Mutex
	samples []float64
	since   time.Time
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool, metrics: metrics.NewStore(pool)}
}

// record garde la dernière mesure et rend l'état de la fenêtre.
func (store *Store) record(latency float64) Database {
	store.mutex.Lock()
	defer store.mutex.Unlock()

	if store.since.IsZero() {
		store.since = time.Now().UTC()
	}
	store.samples = append(store.samples, latency)
	if len(store.samples) > latencySamples {
		store.samples = store.samples[len(store.samples)-latencySamples:]
	}

	maximum := 0.0
	for _, sample := range store.samples {
		if sample > maximum {
			maximum = sample
		}
	}
	return Database{
		LatencyMilliseconds:        latency,
		MaximumLatencyMilliseconds: maximum,
		Samples:                    append([]float64(nil), store.samples...),
		MeasuredSince:              store.since,
	}
}

func (store *Store) Snapshot(ctx context.Context) (Snapshot, error) {
	// Le ping était déjà là pour savoir si la base répond ; le chronométrer
	// répond en plus à « à quelle vitesse ».
	start := time.Now()
	if err := store.pool.Ping(ctx); err != nil {
		return Snapshot{}, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	database := store.record(round(float64(time.Since(start).Microseconds()) / 1000))

	var activeWorkers int
	var lastWorkerSeen *time.Time
	if err := store.pool.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE last_seen_at >= now() - make_interval(secs => $1)),
			max(last_seen_at)
		FROM cairnops_component_heartbeats
		WHERE component = 'worker'
	`, int(workerFreshness.Seconds())).Scan(&activeWorkers, &lastWorkerSeen); err != nil {
		return Snapshot{}, fmt.Errorf("read worker health: %w", err)
	}

	checkedAt := time.Now().UTC()
	worker := workerComponent(activeWorkers, lastWorkerSeen)
	overall := "operational"
	if worker.Status != StatusOperational {
		overall = "degraded"
	}

	// Une série horaire illisible ne vide pas la page : les composants et la
	// base restent lisibles, les micro-graphes se taisent.
	hours, err := store.metrics.InstanceHours(ctx, instanceHours)
	if err != nil {
		hours = []metrics.Hour{}
	}

	return Snapshot{
		Status:    overall,
		CheckedAt: checkedAt,
		Components: []Component{
			{Name: "server", Status: StatusOperational, Instances: 1},
			worker,
			{Name: "postgresql", Status: StatusOperational, Instances: 1},
		},
		Database: database,
		Hours:    hours,
	}, nil
}

// round garde deux décimales : sous la milliseconde, la précision affichée
// serait celle du bruit.
func round(value float64) float64 {
	return float64(int(value*100+0.5)) / 100
}

func workerComponent(activeWorkers int, lastSeen *time.Time) Component {
	status := StatusOperational
	if activeWorkers == 0 && lastSeen != nil {
		status = StatusStale
	}
	if activeWorkers == 0 && lastSeen == nil {
		status = StatusUnavailable
	}
	return Component{Name: "worker", Status: status, Instances: activeWorkers, LastSeenAt: lastSeen}
}

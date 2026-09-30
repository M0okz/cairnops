package latency

import (
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
)

// Une Observation défavorable porte le délai écoulé avant son échec, souvent le
// délai d'attente entier. Le Profil ne doit pas apprendre qu'un service
// indisponible est un service lent.
func TestPostgresLatencyProfileLearnsOnHealthyObservationsAlone(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	store := NewStore(pool)
	targetID, sourceID := seedNativeSource(t, ctx, pool, "http")

	insertObservations(t, ctx, pool, targetID, sourceID, 1, 60, 100, domain.OutcomeHealthy)
	insertObservations(t, ctx, pool, targetID, sourceID, 1, 60, 5000, domain.OutcomeUnhealthy)
	insertObservations(t, ctx, pool, targetID, sourceID, 1, 40, 9000, domain.OutcomeUnknown)

	mustRebuild(t, ctx, store, sourceID)

	profile := mustProfile(t, ctx, store, sourceID)
	if profile.Samples != 60 {
		t.Fatalf("expected the 60 healthy observations alone, got %d", profile.Samples)
	}
	overall, found := profile.Bucket(domain.AllHours)
	if !found {
		t.Fatal("expected an all-hours bucket")
	}
	if overall.Median != 100*time.Millisecond {
		t.Fatalf("expected a median of 100ms, got %s", overall.Median)
	}
	if overall.P99 != 100*time.Millisecond {
		t.Fatalf("expected an untainted high quantile, got %s", overall.P99)
	}
}

// La latence habituelle d'un service dépend de l'heure. C'est la saisonnalité
// que le Profil existe pour reconnaître, et un seuil unique l'ignorerait.
func TestPostgresLatencyProfileSeparatesTheHours(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	store := NewStore(pool)
	targetID, sourceID := seedNativeSource(t, ctx, pool, "http")

	quiet := insertObservations(t, ctx, pool, targetID, sourceID, 3, 40, 80, domain.OutcomeHealthy)
	busy := insertObservations(t, ctx, pool, targetID, sourceID, 1, 40, 800, domain.OutcomeHealthy)

	mustRebuild(t, ctx, store, sourceID)
	profile := mustProfile(t, ctx, store, sourceID)

	quietBucket, found := profile.Bucket(quiet)
	if !found {
		t.Fatalf("expected a bucket for the quiet hour %d", quiet)
	}
	busyBucket, found := profile.Bucket(busy)
	if !found {
		t.Fatalf("expected a bucket for the busy hour %d", busy)
	}
	if quietBucket.Median != 80*time.Millisecond || busyBucket.Median != 800*time.Millisecond {
		t.Fatalf("each hour keeps its own habit: %s and %s", quietBucket.Median, busyBucket.Median)
	}

	// Le seuil suit l'heure : ce qui est lent le matin ne l'est pas le soir.
	quietThreshold, ok := quietBucket.Threshold()
	if !ok {
		t.Fatal("expected a threshold on the quiet hour")
	}
	busyThreshold, ok := busyBucket.Threshold()
	if !ok {
		t.Fatal("expected a threshold on the busy hour")
	}
	if !(busyThreshold > quietThreshold) {
		t.Fatalf("the busy hour must tolerate more, got %s against %s", busyThreshold, quietThreshold)
	}

	// Toutes heures confondues, le seau de repli réunit les deux.
	overall, found := profile.Bucket(domain.AllHours)
	if !found || overall.Samples != 80 {
		t.Fatalf("expected an all-hours bucket of 80 observations, got %#v", overall)
	}
}

// Un Profil sans Observation saine existe avec zéro échantillon : CairnOps a
// regardé et n'a rien établi. Il ne se recalcule donc pas en boucle.
func TestPostgresLatencyProfileRecordsHavingFoundNothing(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	store := NewStore(pool)
	targetID, sourceID := seedNativeSource(t, ctx, pool, "http")
	insertObservations(t, ctx, pool, targetID, sourceID, 1, 40, 5000, domain.OutcomeUnhealthy)

	mustRebuild(t, ctx, store, sourceID)

	profile := mustProfile(t, ctx, store, sourceID)
	if profile.Samples != 0 {
		t.Fatalf("expected no sample, got %d", profile.Samples)
	}
	if len(profile.Buckets) != 0 {
		t.Fatalf("expected no bucket, got %#v", profile.Buckets)
	}
	if _, found := profile.ThresholdAt(time.Now()); found {
		t.Fatal("an empty profile concludes nothing")
	}

	due, err := store.DueSources(ctx, time.Now().UTC().Add(-time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if contains(due, sourceID) {
		t.Fatal("a freshly computed profile is not due again")
	}
}

// Seuls les Contrôles natifs qui mesurent la réactivité d'une Cible ont un
// Profil. Un Heartbeat mesure le temps que CairnOps met à évaluer son
// échéance ; une Source d'Intégration rapporte ce que son produit veut bien.
func TestPostgresLatencyProfileCoversNativeLatencyChecksAlone(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	store := NewStore(pool)

	targetID, httpSourceID := seedNativeSource(t, ctx, pool, "http")
	_, heartbeatSourceID := seedNativeSource(t, ctx, pool, "heartbeat")
	integrationSourceID := seedIntegrationSource(t, ctx, pool, targetID)

	due, err := store.DueSources(ctx, time.Now().UTC(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(due, httpSourceID) {
		t.Fatal("a native HTTP control measures latency and is due")
	}
	if contains(due, heartbeatSourceID) {
		t.Fatal("a heartbeat does not measure the target's responsiveness")
	}
	if contains(due, integrationSourceID) {
		t.Fatal("an integration source carries the latency of its own product")
	}
}

// Une Source suspendue n'apprend plus : son Profil vieillit avec elle plutôt
// que de se recalculer sur une fenêtre qui se vide.
func TestPostgresLatencyProfileLeavesADisabledSourceAlone(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	store := NewStore(pool)
	_, sourceID := seedNativeSource(t, ctx, pool, "http")
	if _, err := pool.Exec(ctx, `UPDATE cairnops_signal_sources SET enabled = false WHERE id = $1::uuid`, sourceID); err != nil {
		t.Fatal(err)
	}

	due, err := store.DueSources(ctx, time.Now().UTC(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if contains(due, sourceID) {
		t.Fatal("a disabled source is not due")
	}
}

// La fenêtre est glissante : un seau qu'elle a quitté doit disparaître, sans
// quoi le Profil décrirait indéfiniment une heure qu'il n'observe plus.
func TestPostgresLatencyProfileRewritesItsBucketsCompletely(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	store := NewStore(pool)
	targetID, sourceID := seedNativeSource(t, ctx, pool, "http")
	departed := insertObservations(t, ctx, pool, targetID, sourceID, 1, 40, 100, domain.OutcomeHealthy)

	mustRebuild(t, ctx, store, sourceID)
	if _, found := mustProfile(t, ctx, store, sourceID).Bucket(departed); !found {
		t.Fatalf("expected a bucket for hour %d", departed)
	}

	// Les Observations quittent la fenêtre, et le Profil se recalcule.
	if _, err := pool.Exec(ctx, `DELETE FROM cairnops_observations WHERE source_id = $1::uuid`, sourceID); err != nil {
		t.Fatal(err)
	}
	remaining := insertObservations(t, ctx, pool, targetID, sourceID, 2, 40, 100, domain.OutcomeHealthy)
	mustRebuild(t, ctx, store, sourceID)

	profile := mustProfile(t, ctx, store, sourceID)
	if _, found := profile.Bucket(departed); found {
		t.Fatalf("hour %d left the window and must leave the profile", departed)
	}
	if _, found := profile.Bucket(remaining); !found {
		t.Fatalf("expected the newly observed hour %d", remaining)
	}
}

// Le Profil se calcule sur les Observations encore présentes, pas sur celles
// qu'il faudrait conserver pour lui.
func TestPostgresLatencyProfileIgnoresObservationsOutsideItsWindow(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	store := NewStore(pool)
	targetID, sourceID := seedNativeSource(t, ctx, pool, "http")

	insertObservations(t, ctx, pool, targetID, sourceID, 1, 40, 100, domain.OutcomeHealthy)
	insertObservationsDaysAgo(t, ctx, pool, targetID, sourceID, 30, 40, 9000)

	mustRebuild(t, ctx, store, sourceID)

	profile := mustProfile(t, ctx, store, sourceID)
	if profile.Samples != 40 {
		t.Fatalf("expected the 40 observations inside the window, got %d", profile.Samples)
	}
	overall, _ := profile.Bucket(domain.AllHours)
	if overall.Median != 100*time.Millisecond {
		t.Fatalf("expected a median of 100ms, got %s", overall.Median)
	}
	if !profile.WindowStart.Before(profile.WindowEnd) {
		t.Fatalf("a profile carries its window: %s to %s", profile.WindowStart, profile.WindowEnd)
	}
}

// Recalculer deux fois de suite ne duplique rien et ne change rien.
func TestPostgresLatencyProfileRebuildsIdempotently(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	store := NewStore(pool)
	targetID, sourceID := seedNativeSource(t, ctx, pool, "http")
	insertObservations(t, ctx, pool, targetID, sourceID, 1, 40, 100, domain.OutcomeHealthy)

	mustRebuild(t, ctx, store, sourceID)
	first := mustProfile(t, ctx, store, sourceID)
	mustRebuild(t, ctx, store, sourceID)
	second := mustProfile(t, ctx, store, sourceID)

	if len(first.Buckets) != len(second.Buckets) {
		t.Fatalf("expected %d buckets again, got %d", len(first.Buckets), len(second.Buckets))
	}
	if first.Samples != second.Samples {
		t.Fatalf("expected %d samples again, got %d", first.Samples, second.Samples)
	}
}

// Une Source sans Profil calculé ne rend rien plutôt qu'un Profil vide : rien
// n'est inventé en attendant la première passe.
func TestPostgresLatencyProfileStaysAbsentBeforeItsFirstPass(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	store := NewStore(pool)
	_, sourceID := seedNativeSource(t, ctx, pool, "http")

	if _, err := store.Profile(ctx, sourceID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	due, err := store.DueSources(ctx, time.Now().UTC(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(due, sourceID) {
		t.Fatal("a source without a profile is due")
	}
}

// Le détail d'une Cible lit les Profils de ses Sources en une fois.
func TestPostgresLatencyProfilesReadPerTarget(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	store := NewStore(pool)
	targetID, sourceID := seedNativeSource(t, ctx, pool, "http")
	insertObservations(t, ctx, pool, targetID, sourceID, 1, 40, 250, domain.OutcomeHealthy)
	mustRebuild(t, ctx, store, sourceID)

	profiles, err := store.Profiles(ctx, targetID)
	if err != nil {
		t.Fatal(err)
	}
	profile, found := profiles[sourceID]
	if !found {
		t.Fatalf("expected the profile of source %s", sourceID)
	}
	if len(profile.Buckets) == 0 {
		t.Fatal("expected the profile to carry its buckets")
	}
	overall, found := profile.Bucket(domain.AllHours)
	if !found || overall.Median != 250*time.Millisecond {
		t.Fatalf("unexpected all-hours bucket: %#v", overall)
	}
}

// Le Profil disparaît avec la Source qui l'a appris.
func TestPostgresLatencyProfileDisappearsWithItsSource(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	store := NewStore(pool)
	targetID, sourceID := seedNativeSource(t, ctx, pool, "http")
	insertObservations(t, ctx, pool, targetID, sourceID, 1, 40, 100, domain.OutcomeHealthy)
	mustRebuild(t, ctx, store, sourceID)

	if _, err := pool.Exec(ctx, `DELETE FROM cairnops_signal_sources WHERE id = $1::uuid`, sourceID); err != nil {
		t.Fatal(err)
	}

	var remaining int
	if err := pool.QueryRow(ctx, `
		SELECT count(*)::integer FROM cairnops_source_latency_hours WHERE source_id = $1::uuid
	`, sourceID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("expected the buckets to be gone, got %d", remaining)
	}
}

// La passe du worker reprend les Sources dues et rend leur nombre.
func TestPostgresLatencyBuilderComputesDueProfiles(t *testing.T) {
	ctx, pool := openTestDatabase(t)
	targetID, sourceID := seedNativeSource(t, ctx, pool, "http")
	insertObservations(t, ctx, pool, targetID, sourceID, 1, 40, 100, domain.OutcomeHealthy)

	builder := NewBuilder(pool, discardLogger())
	built, err := builder.Build(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if built < 1 {
		t.Fatalf("expected at least one profile to be computed, got %d", built)
	}
	if _, err := NewStore(pool).Profile(ctx, sourceID); err != nil {
		t.Fatalf("expected a persisted profile: %v", err)
	}

	// Une seconde passe immédiate ne recalcule rien : les Profils sont frais.
	built, err = builder.Build(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if built != 0 {
		t.Fatalf("expected no recomputation, got %d", built)
	}
}

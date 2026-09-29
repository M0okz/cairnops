package domain

import (
	"testing"
	"time"
)

// Un seau qui ne réunit pas assez d'Observations ne fournit pas de seuil :
// quelques mesures décrivent la dernière heure, pas une habitude.
func TestLatencyBucketWithoutEnoughSamplesEstablishesNoThreshold(t *testing.T) {
	bucket := LatencyBucket{Hour: 9, Samples: MinimumLatencySamples - 1, Median: 100 * time.Millisecond, P99: 300 * time.Millisecond}

	if threshold, found := bucket.Threshold(); found {
		t.Fatalf("expected no threshold, got %s", threshold)
	}
}

// Une Source dispersée conserve sa propre dispersion : son quantile haut
// dépasse les deux bornes et sert de seuil tel quel.
func TestLatencyBucketKeepsItsOwnSpread(t *testing.T) {
	bucket := LatencyBucket{
		Hour: 9, Samples: 400,
		Median: 100 * time.Millisecond, P95: 600 * time.Millisecond, P99: 900 * time.Millisecond,
	}

	threshold, found := bucket.Threshold()
	if !found {
		t.Fatal("expected a threshold")
	}
	if threshold != 900*time.Millisecond {
		t.Fatalf("expected the high quantile of 900ms, got %s", threshold)
	}
}

// Une Source très régulière verrait une variation ordinaire franchir son seul
// quantile haut. Le seuil exige alors un écart franc à la médiane.
func TestLatencyBucketRequiresAClearGapOnASteadySource(t *testing.T) {
	bucket := LatencyBucket{
		Hour: 9, Samples: 400,
		Median: 400 * time.Millisecond, P95: 405 * time.Millisecond, P99: 410 * time.Millisecond,
	}

	threshold, found := bucket.Threshold()
	if !found {
		t.Fatal("expected a threshold")
	}
	if threshold != 800*time.Millisecond {
		t.Fatalf("expected twice the median, got %s", threshold)
	}
}

// Sur une Source très rapide, doubler la médiane resterait dans le bruit d'un
// réseau local. La marge absolue prend alors le relais.
func TestLatencyBucketAddsAnAbsoluteMarginOnAFastSource(t *testing.T) {
	bucket := LatencyBucket{
		Hour: 9, Samples: 400,
		Median: 4 * time.Millisecond, P95: 6 * time.Millisecond, P99: 8 * time.Millisecond,
	}

	threshold, found := bucket.Threshold()
	if !found {
		t.Fatal("expected a threshold")
	}
	if threshold != 54*time.Millisecond {
		t.Fatalf("expected the median plus the absolute margin, got %s", threshold)
	}
}

// L'heure concernée prime sur le repli : c'est elle qui porte la saisonnalité
// que le Profil existe pour reconnaître.
func TestLatencyProfilePrefersTheHourOfTheInstant(t *testing.T) {
	profile := LatencyProfile{Buckets: []LatencyBucket{
		{Hour: AllHours, Samples: 5000, Median: 100 * time.Millisecond, P99: 300 * time.Millisecond},
		{Hour: 3, Samples: 200, Median: 800 * time.Millisecond, P99: 2000 * time.Millisecond},
	}}

	bucket, found := profile.BucketAt(time.Date(2026, 9, 29, 3, 42, 0, 0, time.UTC))
	if !found {
		t.Fatal("expected a bucket")
	}
	if bucket.Hour != 3 {
		t.Fatalf("expected the bucket of the third hour, got %d", bucket.Hour)
	}
}

// Une Source à cadence lente ne remplit pas chaque heure. Le seau de toutes
// les heures lui évite d'attendre un Profil complet.
func TestLatencyProfileFallsBackToAllHoursOnAThinHour(t *testing.T) {
	profile := LatencyProfile{Buckets: []LatencyBucket{
		{Hour: AllHours, Samples: 700, Median: 100 * time.Millisecond, P99: 300 * time.Millisecond},
		{Hour: 3, Samples: MinimumLatencySamples - 1, Median: 800 * time.Millisecond, P99: 2000 * time.Millisecond},
	}}

	bucket, found := profile.BucketAt(time.Date(2026, 9, 29, 3, 42, 0, 0, time.UTC))
	if !found {
		t.Fatal("expected the fallback bucket")
	}
	if bucket.Hour != AllHours {
		t.Fatalf("expected the all-hours bucket, got %d", bucket.Hour)
	}
}

// Sans seau assez fourni, le Profil ne conclut rien plutôt que de conclure sur
// une habitude qu'il n'a pas établie.
func TestLatencyProfileConcludesNothingWithoutAnyRichEnoughBucket(t *testing.T) {
	profile := LatencyProfile{Buckets: []LatencyBucket{
		{Hour: AllHours, Samples: MinimumLatencySamples - 1, Median: 100 * time.Millisecond, P99: 300 * time.Millisecond},
		{Hour: 3, Samples: 2, Median: 800 * time.Millisecond, P99: 2000 * time.Millisecond},
	}}

	if _, found := profile.BucketAt(time.Date(2026, 9, 29, 3, 42, 0, 0, time.UTC)); found {
		t.Fatal("expected no bucket")
	}
	if _, found := profile.ThresholdAt(time.Date(2026, 9, 29, 3, 42, 0, 0, time.UTC)); found {
		t.Fatal("expected no threshold")
	}
}

// Une heure vide n'est pas une heure à zéro : le Profil ne prétend rien sur ce
// qu'il n'a pas observé.
func TestLatencyProfileLeavesAnUnobservedHourAbsent(t *testing.T) {
	profile := LatencyProfile{Buckets: []LatencyBucket{{Hour: 3, Samples: 200}}}

	if _, found := profile.Bucket(4); found {
		t.Fatal("expected the fourth hour to stay absent")
	}
}

// Les seaux sont ceux d'UTC, comme tout horodatage stocké. Un instant exprimé
// dans un autre fuseau retrouve donc le même seau.
func TestLatencyProfileBucketsOnUTCHours(t *testing.T) {
	profile := LatencyProfile{Buckets: []LatencyBucket{
		{Hour: 14, Samples: 400, Median: 100 * time.Millisecond, P99: 900 * time.Millisecond},
	}}
	elsewhere := time.FixedZone("UTC+2", 2*60*60)

	threshold, found := profile.ThresholdAt(time.Date(2026, 9, 29, 16, 5, 0, 0, elsewhere))
	if !found {
		t.Fatal("expected the threshold of the fourteenth UTC hour")
	}
	if threshold != 900*time.Millisecond {
		t.Fatalf("expected 900ms, got %s", threshold)
	}
}

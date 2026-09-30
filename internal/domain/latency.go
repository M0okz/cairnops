package domain

import "time"

const (
	// LatencyProfileWindow est la profondeur sur laquelle un Profil de latence
	// s'apprend. Elle reste inférieure à la rétention des Observations brutes :
	// le Profil se calcule sur ce qui existe encore, jamais sur ce qu'il
	// faudrait conserver pour lui.
	LatencyProfileWindow = 28 * 24 * time.Hour

	// LatencyEvaluationWindow sépare les Observations évaluées de celles qui
	// apprennent le Profil. Une hausse récente ne doit pas normaliser son
	// propre seuil pendant l'évaluation.
	LatencyEvaluationWindow = 24 * time.Hour

	// MinimumLatencySamples est le nombre d'Observations saines en dessous
	// duquel un seau ne fournit aucun seuil. Quelques Observations décrivent la
	// dernière heure, pas une habitude.
	MinimumLatencySamples = 30

	// LatencyDeviationFactor et MinimumLatencyMargin bornent le seuil par le
	// bas. Le seul quantile haut rendrait une Source très régulière
	// hypersensible : son quantile est proche de sa médiane, et une variation
	// ordinaire le franchirait. Une réponse lente doit s'écarter franchement de
	// l'habitude, en proportion comme en valeur absolue.
	LatencyDeviationFactor = 2
	MinimumLatencyMargin   = 50 * time.Millisecond
)

// LatencyObservation est une mesure saine récente à évaluer hors entraînement.
type LatencyObservation struct {
	ID         string
	SourceID   string
	ObservedAt time.Time
	Latency    time.Duration
}

// LatencyAnomaly est une Observation saine exceptionnellement lente au regard
// du Profil appris avant sa fenêtre. Elle n'est ni une Observation défavorable
// ni une Preuve d'Incident.
type LatencyAnomaly struct {
	Observation LatencyObservation
	Median      time.Duration
	Threshold   time.Duration
	Hour        int
	Samples     int
}

// DetectLatencyAnomalies évalue les mesures avec un Profil antérieur. Une
// Source sans seau suffisant ne produit pas de candidat.
func DetectLatencyAnomalies(observations []LatencyObservation, profiles map[string]LatencyProfile) []LatencyAnomaly {
	anomalies := make([]LatencyAnomaly, 0)
	for _, observation := range observations {
		profile, found := profiles[observation.SourceID]
		if !found || observation.ObservedAt.Before(profile.WindowEnd) {
			continue
		}
		bucket, found := profile.BucketAt(observation.ObservedAt)
		if !found {
			continue
		}
		threshold, _ := bucket.Threshold()
		if observation.Latency <= threshold {
			continue
		}
		anomalies = append(anomalies, LatencyAnomaly{
			Observation: observation,
			Median:      bucket.Median,
			Threshold:   threshold,
			Hour:        bucket.Hour,
			Samples:     bucket.Samples,
		})
	}
	return anomalies
}

// AllHours désigne le seau qui réunit toutes les heures de la fenêtre. Il sert
// de repli aux Sources dont la cadence ne remplit pas chaque heure.
const AllHours = -1

// LatencyBucket est la latence habituelle d'une Source sur une heure UTC de la
// fenêtre, ou sur toutes ses heures confondues lorsque Hour vaut AllHours.
//
// Il ne retient que des quantiles : une moyenne et un écart type suivraient les
// valeurs extrêmes que le seau est justement censé aider à reconnaître.
type LatencyBucket struct {
	Hour    int
	Samples int
	Median  time.Duration
	P95     time.Duration
	P99     time.Duration
}

// Threshold rend le seuil au-delà duquel une latence cesse d'être habituelle,
// et dit si le seau permet de l'établir.
//
// Aucune de ces bornes n'est apprise : le seau fournit les quantiles, le
// domaine nomme les marges. Un seau trop pauvre ne conclut rien plutôt que de
// conclure mal.
func (bucket LatencyBucket) Threshold() (time.Duration, bool) {
	if bucket.Samples < MinimumLatencySamples {
		return 0, false
	}
	return max(
		bucket.P99,
		bucket.Median*LatencyDeviationFactor,
		bucket.Median+MinimumLatencyMargin,
	), true
}

// LatencyProfile est le Profil de latence d'une Source : ce qu'elle a mesuré
// d'habituel sur sa fenêtre, heure par heure, et la provenance de ce calcul.
//
// Il décrit, il ne conclut pas. Aucune de ses valeurs n'est un score : elles
// se lisent en latences, dans l'unité déjà affichée partout ailleurs.
type LatencyProfile struct {
	SourceID    string
	WindowStart time.Time
	WindowEnd   time.Time
	Samples     int
	ComputedAt  time.Time
	Buckets     []LatencyBucket
}

// Bucket retrouve un seau par son heure. Un seau manquant n'est pas un seau
// vide : le Profil ne prétend rien sur une heure qu'il n'a pas observée.
func (profile LatencyProfile) Bucket(hour int) (LatencyBucket, bool) {
	for _, bucket := range profile.Buckets {
		if bucket.Hour == hour {
			return bucket, true
		}
	}
	return LatencyBucket{}, false
}

// BucketAt choisit le seau qui décrit un instant : celui de son heure UTC
// lorsqu'il est assez fourni, sinon celui de toutes les heures confondues.
//
// Ce repli laisse une Source à cadence lente disposer d'un Profil sans
// attendre que chacune de ses vingt-quatre heures se remplisse. Si aucun des
// deux n'y parvient, le Profil reste absent pour cet instant.
func (profile LatencyProfile) BucketAt(at time.Time) (LatencyBucket, bool) {
	if bucket, found := profile.Bucket(at.UTC().Hour()); found && bucket.Samples >= MinimumLatencySamples {
		return bucket, true
	}
	if bucket, found := profile.Bucket(AllHours); found && bucket.Samples >= MinimumLatencySamples {
		return bucket, true
	}
	return LatencyBucket{}, false
}

// ThresholdAt rend le seuil applicable à un instant, et dit si le Profil
// permet de l'établir. Sans seuil, rien n'est conclu : l'absence de preuve
// n'établit jamais qu'une latence est normale.
func (profile LatencyProfile) ThresholdAt(at time.Time) (time.Duration, bool) {
	bucket, found := profile.BucketAt(at)
	if !found {
		return 0, false
	}
	return bucket.Threshold()
}

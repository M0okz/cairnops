package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/domain"
	"github.com/M0okz/cairnops/internal/metrics"
)

const profiledTargetID = "4f2a1c58-6f1f-4a1c-9a41-9d2f0b1c8e77"
const profiledSourceID = "9a1d0f28-6d3b-4f52-8f0e-2c1a5d7b3e94"

type fakeLatencyProfiles struct {
	profiles map[string]domain.LatencyProfile
	err      error
}

func (fake fakeLatencyProfiles) Profiles(context.Context, string) (map[string]domain.LatencyProfile, error) {
	return fake.profiles, fake.err
}

func profiledDetail() metrics.TargetDetail {
	return metrics.TargetDetail{
		TargetID: profiledTargetID,
		Measures: []domain.Measure{{Window: domain.WindowDay}},
		Sources:  []metrics.SourceMetrics{{SourceID: profiledSourceID, Name: "Endpoint public"}},
	}
}

func readTargetMetrics(t *testing.T, options ServerOptions) (int, string) {
	t.Helper()
	server := NewServer(options)
	request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/targets/%s/metrics", profiledTargetID), nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	return response.Code, response.Body.String()
}

// Le Profil se lit en millisecondes, comme toute latence de l'API, et porte le
// seuil au-delà duquel une latence cesse d'être habituelle.
func TestTargetMetricsCarriesTheLatencyProfileInMilliseconds(t *testing.T) {
	t.Parallel()

	profiles := fakeLatencyProfiles{profiles: map[string]domain.LatencyProfile{
		profiledSourceID: {
			SourceID: profiledSourceID,
			Samples:  400,
			Buckets: []domain.LatencyBucket{
				{Hour: domain.AllHours, Samples: 400, Median: 100 * time.Millisecond, P95: 250 * time.Millisecond, P99: 900 * time.Millisecond},
			},
		},
	}}
	code, body := readTargetMetrics(t, ServerOptions{
		Identity: &fakeIdentity{}, Metrics: fakeMetrics{detail: profiledDetail()}, LatencyProfiles: profiles,
	})

	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", code, body)
	}
	var payload struct {
		LatencyProfiles map[string]latencyProfileView `json:"latency_profiles"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatal(err)
	}
	profile, found := payload.LatencyProfiles[profiledSourceID]
	if !found {
		t.Fatalf("expected the profile of the source: %s", body)
	}
	if len(profile.Hours) != 1 {
		t.Fatalf("expected one hour, got %#v", profile.Hours)
	}
	hour := profile.Hours[0]
	if hour.MedianMilliseconds != 100 || hour.P95Milliseconds != 250 || hour.P99Milliseconds != 900 {
		t.Fatalf("unexpected quantiles: %#v", hour)
	}
	if hour.ThresholdMilliseconds == nil || *hour.ThresholdMilliseconds != 900 {
		t.Fatalf("expected a threshold of 900ms, got %v", hour.ThresholdMilliseconds)
	}
}

// Une heure trop pauvre ne porte pas de seuil : le client doit pouvoir
// distinguer une heure sur laquelle rien n'est établi d'une heure tolérante.
func TestTargetMetricsLeavesAThinHourWithoutThreshold(t *testing.T) {
	t.Parallel()

	profiles := fakeLatencyProfiles{profiles: map[string]domain.LatencyProfile{
		profiledSourceID: {
			SourceID: profiledSourceID,
			Samples:  4,
			Buckets: []domain.LatencyBucket{
				{Hour: 7, Samples: 4, Median: 100 * time.Millisecond, P95: 120 * time.Millisecond, P99: 130 * time.Millisecond},
			},
		},
	}}
	code, body := readTargetMetrics(t, ServerOptions{
		Identity: &fakeIdentity{}, Metrics: fakeMetrics{detail: profiledDetail()}, LatencyProfiles: profiles,
	})

	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", code, body)
	}
	if !strings.Contains(body, `"threshold_milliseconds":null`) {
		t.Fatalf("a thin hour establishes no threshold: %s", body)
	}
}

// Un Profil est une habitude apprise, pas une mesure. Son indisponibilité ne
// doit pas rendre le détail d'une Cible inconsultable.
func TestTargetMetricsSurvivesAFailingLatencyProfileRead(t *testing.T) {
	t.Parallel()

	code, body := readTargetMetrics(t, ServerOptions{
		Identity: &fakeIdentity{}, Metrics: fakeMetrics{detail: profiledDetail()},
		LatencyProfiles: fakeLatencyProfiles{err: fmt.Errorf("database is unreachable")},
	})

	if code != http.StatusOK {
		t.Fatalf("expected the measures to remain readable, got %d: %s", code, body)
	}
	if !strings.Contains(body, `"name":"Endpoint public"`) {
		t.Fatalf("the measures must still be served: %s", body)
	}
	if !strings.Contains(body, `"latency_profiles":{}`) {
		t.Fatalf("expected no profile rather than a wrong one: %s", body)
	}
}

// Une installation dont aucun Profil n'a encore été calculé répond un
// ensemble vide plutôt qu'une absence de champ.
func TestTargetMetricsCarriesAnEmptyProfileSetBeforeTheFirstPass(t *testing.T) {
	t.Parallel()

	code, body := readTargetMetrics(t, ServerOptions{
		Identity: &fakeIdentity{}, Metrics: fakeMetrics{detail: profiledDetail()},
	})

	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", code, body)
	}
	if !strings.Contains(body, `"latency_profiles":{}`) {
		t.Fatalf("expected an empty profile set: %s", body)
	}
}

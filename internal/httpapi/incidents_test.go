package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/incidents"
)

type fakeIncidents struct {
	ackActor         string
	invalidateActor  string
	invalidateReason string
	listedTarget     string
	historyDays      int
}

func (*fakeIncidents) List(context.Context, string, int) ([]incidents.Incident, error) {
	return []incidents.Incident{{ID: "10000000-0000-0000-0000-000000000001", Status: "active"}}, nil
}
func (fake *fakeIncidents) ListForTarget(_ context.Context, _ string, targetID string, _ int) ([]incidents.Incident, error) {
	fake.listedTarget = targetID
	return []incidents.Incident{{ID: "10000000-0000-0000-0000-000000000001", Impacts: []incidents.Impact{{TargetID: targetID}}, Status: "resolved"}}, nil
}
func (*fakeIncidents) Get(context.Context, string) (incidents.Incident, error) {
	return incidents.Incident{ID: "10000000-0000-0000-0000-000000000001"}, nil
}
func (fake *fakeIncidents) OpenedByDay(_ context.Context, days int) ([]incidents.OpenedDay, error) {
	fake.historyDays = days
	return []incidents.OpenedDay{{Day: time.Date(2026, time.August, 20, 0, 0, 0, 0, time.UTC), Opened: 3}}, nil
}
func (fake *fakeIncidents) Acknowledge(_ context.Context, incidentID, actorID, _ string) (incidents.Incident, error) {
	fake.ackActor = actorID
	return incidents.Incident{ID: incidentID, AcknowledgementSyncStatus: "synchronized"}, nil
}
func (fake *fakeIncidents) InvalidateEvidence(_ context.Context, incidentID, _ string, actorID, _ string, reason string) (incidents.Incident, error) {
	fake.invalidateActor = actorID
	fake.invalidateReason = reason
	return incidents.Incident{ID: incidentID, Status: "resolved"}, nil
}

func TestIncidentAcknowledgementAllowsOperatorAndUsesAuthenticatedActor(t *testing.T) {
	t.Parallel()
	fake := &fakeIncidents{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "operator"}, Incidents: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/10000000-0000-0000-0000-000000000001/acknowledgement", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || fake.ackActor != "user-id" {
		t.Fatalf("expected operator acknowledgement, status=%d actor=%q body=%s", response.Code, fake.ackActor, response.Body.String())
	}
}

func TestIncidentAcknowledgementRejectsObserver(t *testing.T) {
	t.Parallel()
	fake := &fakeIncidents{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "observer"}, Incidents: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/10000000-0000-0000-0000-000000000001/acknowledgement", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || fake.ackActor != "" {
		t.Fatalf("observer reached acknowledgement, status=%d actor=%q", response.Code, fake.ackActor)
	}
}

func TestEvidenceInvalidationAllowsOperatorAndCapturesReason(t *testing.T) {
	t.Parallel()
	fake := &fakeIncidents{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "operator"}, Incidents: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/10000000-0000-0000-0000-000000000001/evidence/20000000-0000-0000-0000-000000000002/invalidation", strings.NewReader(`{"reason":"La source est en défaut de collecte"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || fake.invalidateActor != "user-id" || fake.invalidateReason != "La source est en défaut de collecte" {
		t.Fatalf("expected motivated operator invalidation, status=%d actor=%q reason=%q body=%s", response.Code, fake.invalidateActor, fake.invalidateReason, response.Body.String())
	}
}

func TestEvidenceInvalidationRejectsObserver(t *testing.T) {
	t.Parallel()
	fake := &fakeIncidents{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "observer"}, Incidents: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/10000000-0000-0000-0000-000000000001/evidence/20000000-0000-0000-0000-000000000002/invalidation", strings.NewReader(`{"reason":"La source est en défaut de collecte"}`))
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || fake.invalidateActor != "" {
		t.Fatalf("observer reached signal invalidation, status=%d actor=%q", response.Code, fake.invalidateActor)
	}
}

func TestIncidentListCanBeScopedToATargetHistory(t *testing.T) {
	t.Parallel()
	fake := &fakeIncidents{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "observer"}, Incidents: fake})
	targetID := "30000000-0000-0000-0000-000000000003"
	request := httptest.NewRequest(http.MethodGet, "/api/v1/incidents?status=all&target_id="+targetID+"&limit=500", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || fake.listedTarget != targetID {
		t.Fatalf("expected target-scoped incident history, status=%d target=%q body=%s", response.Code, fake.listedTarget, response.Body.String())
	}
}

// La série quotidienne partage le préfixe des Incidents : elle doit être
// servie par sa propre route, et non lue comme l'identifiant d'un Incident.
func TestIncidentHistoryIsServedOverTheIncidentIdentifierRoute(t *testing.T) {
	t.Parallel()
	fake := &fakeIncidents{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "observer"}, Incidents: fake})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/history", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || fake.historyDays != 12 {
		t.Fatalf("expected the twelve-day default, status=%d days=%d body=%s", response.Code, fake.historyDays, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"opened":3`) {
		t.Fatalf("expected the daily series, body=%s", response.Body.String())
	}
}

func TestIncidentHistoryRejectsAnUnreadableWindow(t *testing.T) {
	t.Parallel()
	fake := &fakeIncidents{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "observer"}, Incidents: fake})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/history?days=abc", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || fake.historyDays != 0 {
		t.Fatalf("expected a rejected window, status=%d days=%d", response.Code, fake.historyDays)
	}
}

func TestIncidentHistoryRejectsAWindowOutsideTheContract(t *testing.T) {
	t.Parallel()
	fake := &fakeIncidents{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "observer"}, Incidents: fake})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/history?days=91", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || fake.historyDays != 0 {
		t.Fatalf("expected a rejected window, status=%d days=%d", response.Code, fake.historyDays)
	}
}

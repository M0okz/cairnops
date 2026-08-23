package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/reconciliation"
)

const (
	testPrimaryTargetID   = "d4e45e1c-4d14-4fb8-8ddc-8c04ea259214"
	testSecondaryTargetID = "9a1d0f28-6d3b-4f52-8f0e-2c1a5d7b3e94"
)

type fakeReconciliation struct {
	previewCalls int
	enqueued     reconciliation.EnqueueInput
	actor        string
}

func (*fakeReconciliation) ListSuggestions(context.Context, string) ([]reconciliation.Suggestion, error) {
	return []reconciliation.Suggestion{}, nil
}
func (fake *fakeReconciliation) PreviewTargets(_ context.Context, primary, secondary string) (reconciliation.Preview, error) {
	fake.previewCalls++
	return reconciliation.Preview{
		Kind:               "target_merge",
		Primary:            reconciliation.TargetSummary{ID: primary, Name: "Authentik"},
		Secondary:          reconciliation.TargetSummary{ID: secondary, Name: "trust-auth-01"},
		SuggestedPrimaryID: secondary,
		Warnings:           []string{}, Conflicts: []reconciliation.IncidentConflict{},
	}, nil
}
func (*fakeReconciliation) PreviewSourceMove(context.Context, string, string) (reconciliation.Preview, error) {
	return reconciliation.Preview{}, nil
}
func (*fakeReconciliation) ListOperations(context.Context, int) ([]reconciliation.Operation, error) {
	return []reconciliation.Operation{}, nil
}
func (fake *fakeReconciliation) Enqueue(_ context.Context, actor string, input reconciliation.EnqueueInput) (reconciliation.Operation, error) {
	fake.actor, fake.enqueued = actor, input
	return reconciliation.Operation{
		ID: testPrimaryTargetID, Kind: input.Kind, PrimaryTargetID: input.PrimaryTargetID,
		SecondaryTargetID: input.SecondaryTargetID, Status: "queued", Stage: "preparing",
		Preview: map[string]any{}, Result: map[string]any{}, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}, nil
}
func (*fakeReconciliation) RejectSuggestion(context.Context, string, string, string) (reconciliation.Suggestion, error) {
	return reconciliation.Suggestion{}, nil
}
func (*fakeReconciliation) SnoozeSuggestion(context.Context, string, string, reconciliation.SnoozeInput) (reconciliation.Suggestion, error) {
	return reconciliation.Suggestion{}, nil
}
func (*fakeReconciliation) ResolveTarget(_ context.Context, targetID string) (string, error) {
	return targetID, nil
}
func (*fakeReconciliation) ListTargetActivity(context.Context, string, int) ([]reconciliation.TargetActivity, error) {
	return []reconciliation.TargetActivity{}, nil
}

func TestReconciliationQueueIsAdministratorOnly(t *testing.T) {
	t.Parallel()
	fake := &fakeReconciliation{}
	server := NewServer(ServerOptions{
		Identity:        &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "operator"},
		Reconciliations: fake,
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/target-reconciliation/suggestions", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("operator reached reconciliation queue: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestReconciliationPreviewValidatesIdentifiersBeforeService(t *testing.T) {
	t.Parallel()
	fake := &fakeReconciliation{}
	server := NewServer(ServerOptions{
		Identity:        &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "administrator"},
		Reconciliations: fake,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/target-reconciliation/preview", strings.NewReader(`{"primary_target_id":"bad","secondary_target_id":"also-bad"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || fake.previewCalls != 0 {
		t.Fatalf("invalid identifiers reached service: status=%d calls=%d", response.Code, fake.previewCalls)
	}
}

func TestReconciliationEnqueueKeepsExplicitSurvivorAndReason(t *testing.T) {
	t.Parallel()
	fake := &fakeReconciliation{}
	server := NewServer(ServerOptions{
		Identity:        &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "administrator"},
		Reconciliations: fake,
	})
	body := `{"kind":"target_merge","primary_target_id":"` + testPrimaryTargetID + `","secondary_target_id":"` + testSecondaryTargetID + `","reason":"Identité vérifiée par l’administrateur","confirmation":"Authentik"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/target-reconciliation/operations", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()

	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("reconciliation was not queued: status=%d body=%s", response.Code, response.Body.String())
	}
	if fake.enqueued.PrimaryTargetID != testPrimaryTargetID || fake.enqueued.Reason == "" || fake.actor == "" {
		t.Fatalf("explicit decision was lost: actor=%q input=%#v", fake.actor, fake.enqueued)
	}
}

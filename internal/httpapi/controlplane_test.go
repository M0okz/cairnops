package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/controlplane"
)

type fakeControlPlane struct {
	target        controlplane.Target
	archived      string
	restored      string
	deletedSource string
	sourceInput   controlplane.UpdateSourceInput
	err           error
}

func (fake *fakeControlPlane) ListTargets(context.Context) ([]controlplane.Target, error) {
	return []controlplane.Target{fake.target}, nil
}

func (fake *fakeControlPlane) CreateTarget(_ context.Context, input controlplane.CreateTargetInput) (controlplane.Target, error) {
	fake.target = controlplane.Target{
		ID: "d4e45e1c-4d14-4fb8-8ddc-8c04ea259214", Name: input.Name,
		Description: input.Description, CreatedAt: time.Now().UTC(), Sources: []controlplane.Source{},
	}
	return fake.target, nil
}

func (fake *fakeControlPlane) UpdateTarget(_ context.Context, targetID string, input controlplane.UpdateTargetInput) (controlplane.Target, error) {
	if fake.err != nil {
		return controlplane.Target{}, fake.err
	}
	fake.target = controlplane.Target{
		ID: targetID, Name: input.Name, Description: input.Description,
		CreatedAt: time.Now().UTC(), Sources: []controlplane.Source{},
	}
	return fake.target, nil
}

func (fake *fakeControlPlane) ArchiveTarget(_ context.Context, targetID string) error {
	fake.archived = targetID
	return fake.err
}

func (fake *fakeControlPlane) RestoreTarget(_ context.Context, targetID string) (controlplane.Target, error) {
	fake.restored = targetID
	return controlplane.Target{ID: targetID, Sources: []controlplane.Source{}}, fake.err
}

func (*fakeControlPlane) CreateSource(context.Context, string, controlplane.CreateSourceInput) (controlplane.CreatedSource, error) {
	return controlplane.CreatedSource{}, nil
}

func (fake *fakeControlPlane) UpdateSource(_ context.Context, sourceID string, input controlplane.UpdateSourceInput) (controlplane.Source, error) {
	if fake.err != nil {
		return controlplane.Source{}, fake.err
	}
	fake.sourceInput = input
	return controlplane.Source{ID: sourceID, Name: "Endpoint public", Enabled: true}, nil
}

func (fake *fakeControlPlane) DeleteSource(_ context.Context, sourceID string) error {
	fake.deletedSource = sourceID
	return fake.err
}

func (*fakeControlPlane) ListObservations(context.Context, string, int) ([]controlplane.Observation, error) {
	return []controlplane.Observation{}, nil
}

func (*fakeControlPlane) ReceiveHeartbeat(context.Context, string, controlplane.HeartbeatPayload) (controlplane.Observation, error) {
	return controlplane.Observation{ID: 1, Outcome: "healthy"}, nil
}

func TestControlPlaneRequiresAuthentication(t *testing.T) {
	t.Parallel()

	server := NewServer(ServerOptions{
		BootstrapToken: "bootstrap-token-with-at-least-32-characters",
		Identity:       &fakeIdentity{},
		ControlPlane:   &fakeControlPlane{},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/targets", nil)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestControlPlaneRejectsBootstrapTokenAfterInitialization(t *testing.T) {
	t.Parallel()

	const token = "bootstrap-token-with-at-least-32-characters"
	fake := &fakeControlPlane{}
	server := NewServer(ServerOptions{BootstrapToken: token, Identity: &fakeIdentity{}, ControlPlane: fake})
	body, _ := json.Marshal(controlplane.CreateTargetInput{Name: "Nextcloud", Description: "Cloud familial"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/targets", bytes.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", response.Code, response.Body.String())
	}
	if fake.target.Name != "" {
		t.Fatalf("bootstrap token unexpectedly reached the control plane: %#v", fake.target)
	}
}

func TestControlPlaneCreatesTargetWithSession(t *testing.T) {
	t.Parallel()

	fake := &fakeControlPlane{}
	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, ControlPlane: fake})
	body, _ := json.Marshal(controlplane.CreateTargetInput{Name: "Nextcloud", Description: "Cloud familial"})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/targets", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", response.Code, response.Body.String())
	}
	if fake.target.Name != "Nextcloud" {
		t.Fatalf("expected target to be created, got %#v", fake.target)
	}
}

func TestControlPlaneRenamesArchivesAndRestoresATarget(t *testing.T) {
	t.Parallel()

	const targetID = "d4e45e1c-4d14-4fb8-8ddc-8c04ea259214"
	fake := &fakeControlPlane{}
	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, ControlPlane: fake})
	send := func(method, path string, body []byte) *httptest.ResponseRecorder {
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		request := httptest.NewRequest(method, path, reader)
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, request)
		return response
	}

	renamed, _ := json.Marshal(controlplane.UpdateTargetInput{Name: "Nextcloud", Description: "Cloud familial"})
	if response := send(http.MethodPatch, "/api/v1/targets/"+targetID, renamed); response.Code != http.StatusOK {
		t.Fatalf("expected 200 on rename, got %d: %s", response.Code, response.Body.String())
	}
	if fake.target.Name != "Nextcloud" {
		t.Fatalf("the rename never reached the control plane: %#v", fake.target)
	}

	// L'archivage ne rend aucun corps : la Cible sort de l'Espace opérationnel,
	// il n'y a rien à afficher ensuite.
	if response := send(http.MethodDelete, "/api/v1/targets/"+targetID, nil); response.Code != http.StatusNoContent {
		t.Fatalf("expected 204 on archival, got %d: %s", response.Code, response.Body.String())
	}
	if fake.archived != targetID {
		t.Fatalf("the archival never reached the control plane: %q", fake.archived)
	}
	if response := send(http.MethodPost, "/api/v1/targets/"+targetID+"/restoration", nil); response.Code != http.StatusOK {
		t.Fatalf("expected 200 on restoration, got %d: %s", response.Code, response.Body.String())
	}
	if fake.restored != targetID {
		t.Fatalf("the restoration never reached the control plane: %q", fake.restored)
	}
}

// Un champ absent laisse le réglage inchangé : suspendre une sonde ne doit pas
// obliger à réémettre toute sa configuration.
func TestControlPlaneUpdatesOnlyTheSubmittedSourceFields(t *testing.T) {
	t.Parallel()

	const sourceID = "9a1d0f28-6d3b-4f52-8f0e-2c1a5d7b3e94"
	fake := &fakeControlPlane{}
	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, ControlPlane: fake})
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/sources/"+sourceID, bytes.NewReader([]byte(`{"enabled":false}`)))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if fake.sourceInput.Enabled == nil || *fake.sourceInput.Enabled {
		t.Fatalf("the suspension never reached the control plane: %#v", fake.sourceInput)
	}
	if fake.sourceInput.Name != nil || fake.sourceInput.IntervalSeconds != nil || fake.sourceInput.Config != nil {
		t.Fatalf("an absent field must stay absent: %#v", fake.sourceInput)
	}
}

// Une Source apportée par une Intégration ne se règle pas depuis CairnOps.
func TestControlPlaneRefusesToEditAnIntegrationSource(t *testing.T) {
	t.Parallel()

	fake := &fakeControlPlane{err: controlplane.ErrIntegrationOwned}
	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, ControlPlane: fake})
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/sources/9a1d0f28-6d3b-4f52-8f0e-2c1a5d7b3e94",
		bytes.NewReader([]byte(`{"enabled":false}`)))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", response.Code, response.Body.String())
	}
}

func TestHeartbeatAcceptsEmptyBodyWithoutAdminToken(t *testing.T) {
	t.Parallel()

	fake := &fakeControlPlane{}
	server := NewServer(ServerOptions{BootstrapToken: "bootstrap-token-with-at-least-32-characters", Identity: &fakeIdentity{}, ControlPlane: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/heartbeat/UVFRUVFRUVFRUVFRUVFRUVFRUVFRUVFRUVFRUVFRUVE", nil)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", response.Code, response.Body.String())
	}
}

func TestValidUUID(t *testing.T) {
	t.Parallel()

	if !validUUID("d4e45e1c-4d14-4fb8-8ddc-8c04ea259214") {
		t.Fatal("expected UUID to be valid")
	}
	if validUUID("../../etc/passwd") {
		t.Fatal("expected malformed UUID to be rejected")
	}
}

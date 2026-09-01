package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/M0okz/cairnops/internal/bursts"
)

type fakeBursts struct {
	ackActor string
	status   string
}

func (fake *fakeBursts) List(_ context.Context, status string, _ int) ([]bursts.Burst, error) {
	fake.status = status
	return []bursts.Burst{{ID: "10000000-0000-0000-0000-000000000001", Status: "propagating"}}, nil
}

func (*fakeBursts) Get(context.Context, string) (bursts.Burst, error) {
	return bursts.Burst{ID: "10000000-0000-0000-0000-000000000001"}, nil
}

func (fake *fakeBursts) Acknowledge(_ context.Context, burstID, actorID, _ string) (bursts.Burst, error) {
	fake.ackActor = actorID
	return bursts.Burst{ID: burstID, Status: "propagating"}, nil
}

func TestIncidentBurstListAndOperatorAcknowledgement(t *testing.T) {
	t.Parallel()
	fake := &fakeBursts{}
	server := NewServer(ServerOptions{
		Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "operator"},
		Bursts:   fake,
	})

	list := httptest.NewRequest(http.MethodGet, "/api/v1/incident-bursts?status=all", nil)
	list.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	listResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(listResponse, list)
	if listResponse.Code != http.StatusOK || fake.status != "all" {
		t.Fatalf("unexpected burst list: status=%d filter=%q body=%s", listResponse.Code, fake.status, listResponse.Body.String())
	}

	ack := httptest.NewRequest(http.MethodPost, "/api/v1/incident-bursts/10000000-0000-0000-0000-000000000001/acknowledgement", nil)
	ack.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	ackResponse := httptest.NewRecorder()
	server.Handler.ServeHTTP(ackResponse, ack)
	if ackResponse.Code != http.StatusOK || fake.ackActor != "user-id" {
		t.Fatalf("unexpected burst acknowledgement: status=%d actor=%q body=%s", ackResponse.Code, fake.ackActor, ackResponse.Body.String())
	}
}

func TestIncidentBurstAcknowledgementRejectsObserver(t *testing.T) {
	t.Parallel()
	fake := &fakeBursts{}
	server := NewServer(ServerOptions{
		Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "observer"},
		Bursts:   fake,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/incident-bursts/10000000-0000-0000-0000-000000000001/acknowledgement", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || fake.ackActor != "" {
		t.Fatalf("observer reached burst acknowledgement: status=%d actor=%q", response.Code, fake.ackActor)
	}
}

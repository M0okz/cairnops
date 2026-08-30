package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/M0okz/cairnops/internal/indicators"
)

type fakeIndicators struct {
	actorID    string
	connector  string
	userID     string
	pinInput   indicators.PinInput
	applyInput indicators.ApplyInput
}

func (*fakeIndicators) Configuration(context.Context, string) (indicators.Configuration, error) {
	return indicators.Configuration{ConnectorKind: "zabbix", Bindings: []indicators.Binding{}}, nil
}

func (*fakeIndicators) Preview(context.Context, string) (indicators.Configuration, error) {
	return indicators.Configuration{ConnectorKind: "zabbix", Bindings: []indicators.Binding{}}, nil
}

func (fake *fakeIndicators) Apply(_ context.Context, actorID, connectorID string, input indicators.ApplyInput) (indicators.Configuration, error) {
	fake.actorID, fake.connector, fake.applyInput = actorID, connectorID, input
	return indicators.Configuration{ConnectorID: connectorID, Bindings: []indicators.Binding{}}, nil
}

func (fake *fakeIndicators) Overview(_ context.Context, userID string) ([]indicators.TargetProjection, error) {
	fake.userID = userID
	return []indicators.TargetProjection{{TargetID: "target-one", Indicators: []indicators.Indicator{}}}, nil
}

func (fake *fakeIndicators) Catalog(_ context.Context, userID string) ([]indicators.TargetProjection, error) {
	fake.userID = userID
	return []indicators.TargetProjection{{TargetID: "target-one", Indicators: []indicators.Indicator{{ID: "indicator-one"}}}}, nil
}

func (*fakeIndicators) Target(context.Context, string, string, string) (indicators.TargetProjection, error) {
	return indicators.TargetProjection{Indicators: []indicators.Indicator{}}, nil
}

func (*fakeIndicators) Incident(context.Context, string, string) (indicators.IncidentProjection, error) {
	return indicators.IncidentProjection{Indicators: []indicators.Indicator{}, Snapshots: []indicators.Snapshot{}, Series: map[string][]indicators.Point{}}, nil
}

func (*fakeIndicators) Pins(context.Context, string) ([]indicators.Pin, error) {
	return []indicators.Pin{}, nil
}

func (fake *fakeIndicators) SetPins(_ context.Context, userID string, input indicators.PinInput) ([]indicators.Pin, error) {
	fake.userID, fake.pinInput = userID, input
	return []indicators.Pin{{IndicatorID: input.IndicatorIDs[0], Position: 0}}, nil
}

func TestIndicatorConfigurationRequiresAdministratorAndUsesActor(t *testing.T) {
	t.Parallel()
	connectorID := "12345678-1234-4234-8234-123456789012"
	fake := &fakeIndicators{}
	operator := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "operator"}, Indicators: fake})
	request := httptest.NewRequest(http.MethodPut, "/api/v1/connectors/"+connectorID+"/indicator-configuration", bytes.NewBufferString(`{"bindings":[],"profiles":[],"summary":"Test"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	operator.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("operator changed indicator configuration: status=%d body=%s", response.Code, response.Body.String())
	}

	administrator := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "administrator"}, Indicators: fake})
	request = httptest.NewRequest(http.MethodPut, "/api/v1/connectors/"+connectorID+"/indicator-configuration", bytes.NewBufferString(`{"bindings":[],"profiles":[],"summary":"Test"}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response = httptest.NewRecorder()
	administrator.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || fake.actorID != "user-id" || fake.connector != connectorID || fake.applyInput.Summary != "Test" {
		t.Fatalf("unexpected apply: status=%d actor=%q connector=%q input=%#v body=%s", response.Code, fake.actorID, fake.connector, fake.applyInput, response.Body.String())
	}
}

func TestIndicatorOverviewAndPinsArePersonal(t *testing.T) {
	t.Parallel()
	fake := &fakeIndicators{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "observer"}, Indicators: fake})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/indicators/targets", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || fake.userID != "user-id" || !bytes.Contains(response.Body.Bytes(), []byte(`"limit":4`)) {
		t.Fatalf("unexpected overview: status=%d user=%q body=%s", response.Code, fake.userID, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/api/v1/me/indicator-pins", bytes.NewBufferString(`{"indicator_ids":["indicator-one"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || fake.userID != "user-id" || len(fake.pinInput.IndicatorIDs) != 1 || fake.pinInput.IndicatorIDs[0] != "indicator-one" {
		t.Fatalf("unexpected pins: status=%d user=%q input=%#v body=%s", response.Code, fake.userID, fake.pinInput, response.Body.String())
	}
}

func TestIndicatorCatalogIsAvailableToObservers(t *testing.T) {
	t.Parallel()
	fake := &fakeIndicators{}
	server := NewServer(ServerOptions{Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "observer"}, Indicators: fake})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/indicators/catalog", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || fake.userID != "user-id" || !bytes.Contains(response.Body.Bytes(), []byte(`"id":"indicator-one"`)) {
		t.Fatalf("unexpected catalog: status=%d user=%q body=%s", response.Code, fake.userID, response.Body.String())
	}
}

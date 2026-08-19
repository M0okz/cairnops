package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/M0okz/cairnops/internal/notifications"
)

type fakeNotifications struct {
	actor    string
	input    notifications.CreateMattermostInput
	inboxFor string
	readFor  string
	readIDs  []int64
}

func (*fakeNotifications) List(context.Context) ([]notifications.Channel, error) {
	return []notifications.Channel{{ID: "channel-1", Kind: "mattermost", Endpoint: "https://mattermost.example.test"}}, nil
}

func (fake *fakeNotifications) CreateMattermost(_ context.Context, actor string, input notifications.CreateMattermostInput) (notifications.Channel, error) {
	fake.actor, fake.input = actor, input
	return notifications.Channel{ID: "channel-1", Kind: "mattermost", Endpoint: "https://mattermost.example.test"}, nil
}

func (fake *fakeNotifications) Inbox(_ context.Context, userID string, _ int) (notifications.Inbox, error) {
	fake.inboxFor = userID
	return notifications.Inbox{
		Entries: []notifications.InboxEntry{{ID: 1, IncidentID: "incident-1", EventKind: "firing"}},
		Unread:  1,
	}, nil
}

func (fake *fakeNotifications) MarkRead(_ context.Context, userID string, ids []int64) (int, error) {
	fake.readFor, fake.readIDs = userID, ids
	return len(ids), nil
}

func TestMattermostChannelCreationRequiresAdministratorAndDoesNotEchoWebhook(t *testing.T) {
	t.Parallel()
	fake := &fakeNotifications{}
	server := NewServer(ServerOptions{
		Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "administrator"}, Notifications: fake,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notification-channels/mattermost", bytes.NewBufferString(`{"name":"Exploitation","webhook_url":"https://mattermost.example.test/hooks/secret","severities":["major","critical"]}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || fake.actor != "user-id" || fake.input.WebhookURL == "" {
		t.Fatalf("unexpected creation response status=%d actor=%q input=%#v body=%s", response.Code, fake.actor, fake.input, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("/hooks/secret")) {
		t.Fatalf("webhook leaked in API response: %s", response.Body.String())
	}
}

func TestMattermostChannelCreationRejectsOperator(t *testing.T) {
	t.Parallel()
	fake := &fakeNotifications{}
	server := NewServer(ServerOptions{
		Identity: &roleIdentity{fakeIdentity: &fakeIdentity{}, role: "operator"}, Notifications: fake,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notification-channels/mattermost", bytes.NewBufferString(`{}`))
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || fake.actor != "" {
		t.Fatalf("operator reached notification configuration, status=%d actor=%q", response.Code, fake.actor)
	}
}

// La boîte est celle de la session. Aucun rôle n'y donne accès et aucun rôle
// n'en prive : ce ne sont pas des Canaux, ce sont des nouvelles reçues.
func TestInboxServesTheSessionOwnerWhateverTheRole(t *testing.T) {
	t.Parallel()

	for _, role := range []string{"administrator", "operator", "observer"} {
		fake := &fakeNotifications{}
		server := NewServer(ServerOptions{
			Identity:      &roleIdentity{fakeIdentity: &fakeIdentity{}, role: role},
			Notifications: fake,
		})
		request := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
		request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
		response := httptest.NewRecorder()
		server.Handler.ServeHTTP(response, request)

		if response.Code != http.StatusOK {
			t.Fatalf("%s could not read their inbox: status=%d body=%s", role, response.Code, response.Body)
		}
		if fake.inboxFor != "user-id" {
			t.Fatalf("the inbox was not read for the session owner: %q", fake.inboxFor)
		}
		if !strings.Contains(response.Body.String(), `"unread":1`) {
			t.Fatalf("the unread count is missing: %s", response.Body)
		}
	}
}

func TestInboxRequiresASession(t *testing.T) {
	t.Parallel()

	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Notifications: &fakeNotifications{}})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("an anonymous visitor read an inbox: status=%d", response.Code)
	}
}

func TestMarkReadCarriesTheSessionOwnerAndTheChosenEntries(t *testing.T) {
	t.Parallel()

	fake := &fakeNotifications{}
	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Notifications: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read", strings.NewReader(`{"ids":[7,9]}`))
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("the entries were not marked read: status=%d body=%s", response.Code, response.Body)
	}
	if fake.readFor != "user-id" || len(fake.readIDs) != 2 {
		t.Fatalf("the service did not receive the request as written: for=%q ids=%v", fake.readFor, fake.readIDs)
	}

	// Sans corps, tout est lu : c'est le geste courant, et il ne doit pas
	// réclamer une requête plus savante.
	fake.readIDs = nil
	request = httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response = httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || fake.readIDs != nil {
		t.Fatalf("an empty request did not read the whole inbox: status=%d ids=%v", response.Code, fake.readIDs)
	}
}

// L'écriture reste soumise à la même origine, comme tous les gestes qui
// changent l'état de l'instance.
func TestMarkReadRequiresSameOrigin(t *testing.T) {
	t.Parallel()

	fake := &fakeNotifications{}
	server := NewServer(ServerOptions{
		PublicURL: "https://cairnops.example.com", Identity: &fakeIdentity{}, Notifications: fake,
	})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read", nil)
	request.Header.Set("Origin", "https://ailleurs.example.net")
	request.AddCookie(&http.Cookie{Name: "__Host-cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || fake.readFor != "" {
		t.Fatalf("a cross-origin request reached the service: status=%d for=%q", response.Code, fake.readFor)
	}
}

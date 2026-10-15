package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/realtime"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type fakeEventStream struct {
	mu     sync.Mutex
	latest int64
	oldest int64
	events []realtime.Event
}

func (fake *fakeEventStream) OldestVersion(context.Context) (int64, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.oldest, nil
}

func (fake *fakeEventStream) LatestVersion(context.Context) (int64, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return fake.latest, nil
}

func (fake *fakeEventStream) ListAfter(_ context.Context, after int64, limit int) ([]realtime.Event, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	events := make([]realtime.Event, 0, limit)
	for _, event := range fake.events {
		if event.Version > after {
			events = append(events, event)
			if len(events) == limit {
				break
			}
		}
	}
	return events, nil
}

func (fake *fakeEventStream) append(event realtime.Event) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.events = append(fake.events, event)
	fake.latest = event.Version
}

func TestRealtimeRequiresAuthentication(t *testing.T) {
	t.Parallel()

	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Events: &fakeEventStream{}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestRealtimeRejectsInvalidCursorBeforeUpgrade(t *testing.T) {
	t.Parallel()

	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Events: &fakeEventStream{}})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events?after=-1", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestRealtimeStreamsEventsAfterReadyCursor(t *testing.T) {
	stream := &fakeEventStream{latest: 7}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	server := NewServer(ServerOptions{PublicURL: origin, Identity: &fakeIdentity{}, Events: stream})
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-serveErrors
	})

	websocketURL := "ws://" + listener.Addr().String() + "/api/v1/events"
	headers := http.Header{}
	headers.Set("Origin", origin)
	headers.Set("Cookie", (&http.Cookie{Name: "cairnops_session", Value: testSessionToken}).String())
	readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connection, response, err := websocket.Dial(readCtx, websocketURL, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		if response != nil {
			t.Fatalf("dial realtime stream: %v (status %d)", err, response.StatusCode)
		}
		t.Fatalf("dial realtime stream: %v", err)
	}
	defer connection.CloseNow()

	var ready realtimeMessage
	if err := wsjson.Read(readCtx, connection, &ready); err != nil {
		t.Fatalf("read ready message: %v", err)
	}
	if ready.Type != "ready" || ready.Version != 7 {
		t.Fatalf("unexpected ready message: %#v", ready)
	}

	stream.append(realtime.Event{
		Version: 8, Kind: "observation.created", EntityType: "target",
		EntityID: "d4e45e1c-4d14-4fb8-8ddc-8c04ea259214", OccurredAt: time.Now().UTC(),
	})
	var event realtimeMessage
	if err := wsjson.Read(readCtx, connection, &event); err != nil {
		t.Fatalf("read event message: %v", err)
	}
	if event.Type != "event" || event.Version != 8 || event.Kind != "observation.created" {
		t.Fatalf("unexpected event message: %#v", event)
	}
	_ = connection.Close(websocket.StatusNormalClosure, "")
}

// Un curseur que la rétention a dépassé, ou qui vient d'une base restaurée
// depuis, ne se reprend pas : le client apprend qu'il doit recharger une
// projection complète, puis suit le flux depuis la version courante.
func TestRealtimeAsksForResyncWhenCursorCannotResume(t *testing.T) {
	retained := []realtime.Event{
		{Version: 40, Kind: "target.changed", EntityType: "target", EntityID: "a", OccurredAt: time.Now().UTC()},
		{Version: 41, Kind: "target.changed", EntityType: "target", EntityID: "b", OccurredAt: time.Now().UTC()},
	}
	for name, test := range map[string]struct {
		cursor       string
		resync       bool
		readyVersion int64
		firstEvent   int64
	}{
		"cursor just before the oldest event": {cursor: "39", resync: false, readyVersion: 39, firstEvent: 40},
		"cursor before purged events":         {cursor: "12", resync: true, readyVersion: 41, firstEvent: 42},
		"cursor from another database":        {cursor: "900", resync: true, readyVersion: 41, firstEvent: 42},
	} {
		t.Run(name, func(t *testing.T) {
			stream := &fakeEventStream{latest: 41, oldest: 40, events: append([]realtime.Event(nil), retained...)}
			readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			connection := dialRealtime(t, readCtx, stream, "?after="+test.cursor)

			var ready realtimeMessage
			if err := wsjson.Read(readCtx, connection, &ready); err != nil {
				t.Fatalf("read ready message: %v", err)
			}
			if ready.Type != "ready" || ready.Resync != test.resync || ready.Version != test.readyVersion {
				t.Fatalf("unexpected ready message: %#v", ready)
			}
			stream.append(realtime.Event{Version: 42, Kind: "source.changed", EntityType: "source", EntityID: "c", OccurredAt: time.Now().UTC()})
			var event realtimeMessage
			if err := wsjson.Read(readCtx, connection, &event); err != nil {
				t.Fatalf("read event message: %v", err)
			}
			if event.Type != "event" || event.Version != test.firstEvent || event.Resync {
				t.Fatalf("unexpected first event: %#v", event)
			}
		})
	}
}

func dialRealtime(t *testing.T, ctx context.Context, stream EventStream, query string) *websocket.Conn {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	server := NewServer(ServerOptions{PublicURL: origin, Identity: &fakeIdentity{}, Events: stream})
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-serveErrors
	})

	headers := http.Header{}
	headers.Set("Origin", origin)
	headers.Set("Cookie", (&http.Cookie{Name: "cairnops_session", Value: testSessionToken}).String())
	connection, response, err := websocket.Dial(ctx, "ws://"+listener.Addr().String()+"/api/v1/events"+query, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		if response != nil {
			t.Fatalf("dial realtime stream: %v (status %d)", err, response.StatusCode)
		}
		t.Fatalf("dial realtime stream: %v", err)
	}
	t.Cleanup(func() { connection.CloseNow() })
	return connection
}

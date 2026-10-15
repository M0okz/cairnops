package httpapi

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
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

// idleListener établit l'écoute sans jamais notifier : la diffusion repose
// alors sur le sondage de secours, raccourci pour les tests.
type idleListener struct{}

func (idleListener) Listen(ctx context.Context, listening func(), _ func(string)) error {
	listening()
	<-ctx.Done()
	return ctx.Err()
}

func newEventHub(stream *fakeEventStream) *realtime.Hub {
	return realtime.NewHub(stream, idleListener{}, realtime.HubOptions{FallbackInterval: 10 * time.Millisecond})
}

// runEventHub démarre le hub et attend qu'il accepte des abonnements.
func runEventHub(t *testing.T, stream *fakeEventStream) *realtime.Hub {
	t.Helper()
	hub := newEventHub(stream)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		hub.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	awaitEventHub(t, hub)
	return hub
}

func awaitEventHub(t *testing.T, hub *realtime.Hub) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		subscription, err := hub.Subscribe()
		if err == nil {
			subscription.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the event hub never became ready: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRealtimeRequiresAuthentication(t *testing.T) {
	t.Parallel()

	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Events: newEventHub(&fakeEventStream{})})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", response.Code)
	}
}

func TestRealtimeRejectsInvalidCursorBeforeUpgrade(t *testing.T) {
	t.Parallel()

	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Events: newEventHub(&fakeEventStream{})})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events?after=-1", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}

// dialRealtime ouvre le flux d'un serveur complet, authentification comprise.
func dialRealtime(t *testing.T, events EventStream, query string) (*websocket.Conn, context.Context) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin := "http://" + listener.Addr().String()
	server := NewServer(ServerOptions{PublicURL: origin, Identity: &fakeIdentity{}, Events: events})
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-serveErrors
	})

	websocketURL := "ws://" + listener.Addr().String() + "/api/v1/events" + query
	headers := http.Header{}
	headers.Set("Origin", origin)
	headers.Set("Cookie", (&http.Cookie{Name: "cairnops_session", Value: testSessionToken}).String())
	readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	connection, response, err := websocket.Dial(readCtx, websocketURL, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		if response != nil {
			t.Fatalf("dial realtime stream: %v (status %d)", err, response.StatusCode)
		}
		t.Fatalf("dial realtime stream: %v", err)
	}
	t.Cleanup(func() { connection.CloseNow() })
	return connection, readCtx
}

func readRealtime(t *testing.T, ctx context.Context, connection *websocket.Conn) realtimeMessage {
	t.Helper()
	var message realtimeMessage
	if err := wsjson.Read(ctx, connection, &message); err != nil {
		t.Fatalf("read realtime message: %v", err)
	}
	return message
}

func targetEvent(version int64) realtime.Event {
	return realtime.Event{
		Version: version, Kind: "observation.created", EntityType: "target",
		EntityID: "d4e45e1c-4d14-4fb8-8ddc-8c04ea259214", OccurredAt: time.Now().UTC(),
	}
}

func TestRealtimeStreamsEventsAfterReadyCursor(t *testing.T) {
	stream := &fakeEventStream{latest: 7}
	connection, readCtx := dialRealtime(t, runEventHub(t, stream), "")

	if ready := readRealtime(t, readCtx, connection); ready.Type != "ready" || ready.Version != 7 {
		t.Fatalf("unexpected ready message: %#v", ready)
	}
	stream.append(targetEvent(8))
	if event := readRealtime(t, readCtx, connection); event.Type != "event" || event.Version != 8 || event.Kind != "observation.created" {
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
			connection, readCtx := dialRealtime(t, runEventHub(t, stream), "?after="+test.cursor)

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

func TestRealtimeResumeReplaysMissedEventsOnceThenStreams(t *testing.T) {
	stream := &fakeEventStream{}
	for version := int64(1); version <= 5; version++ {
		stream.append(targetEvent(version))
	}
	hub := runEventHub(t, stream)
	connection, readCtx := dialRealtime(t, hub, "?after=2")

	if ready := readRealtime(t, readCtx, connection); ready.Type != "ready" || ready.Version != 2 {
		t.Fatalf("unexpected ready message: %#v", ready)
	}
	stream.append(targetEvent(6))
	// Les versions 3 à 5 viennent du rejeu, la 6 de la diffusion ou du rejeu
	// selon l'instant : chacune doit arriver exactement une fois, dans l'ordre.
	for want := int64(3); want <= 6; want++ {
		if event := readRealtime(t, readCtx, connection); event.Type != "event" || event.Version != want {
			t.Fatalf("expected event %d, got %#v", want, event)
		}
	}
	stream.append(targetEvent(7))
	if event := readRealtime(t, readCtx, connection); event.Version != 7 {
		t.Fatalf("expected event 7, got %#v", event)
	}
}

func TestRealtimeIsUnavailableUntilTheHubIsReady(t *testing.T) {
	t.Parallel()

	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Events: newEventHub(&fakeEventStream{})})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil)
	request.AddCookie(&http.Cookie{Name: "cairnops_session", Value: testSessionToken})
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", response.Code, response.Body.String())
	}
}

// pingedStream sert le flux sans authentification, avec un ping rapide, et
// signale la fin du handler.
func pingedStream(t *testing.T, hub *realtime.Hub) (string, <-chan struct{}) {
	t.Helper()
	handler := realtimeHandler{
		events: hub, logger: slog.New(slog.DiscardHandler),
		pingInterval: 20 * time.Millisecond, pongTimeout: 50 * time.Millisecond,
	}
	ended := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(ended)
		handler.stream(w, r)
	}))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http"), ended
}

func TestRealtimeKeepsAClientThatAnswersPings(t *testing.T) {
	stream := &fakeEventStream{}
	url, ended := pingedStream(t, runEventHub(t, stream))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connection, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer connection.CloseNow()

	// Le client lit en continu, comme un navigateur : c'est ce qui lui fait
	// répondre aux pings.
	messages := make(chan realtimeMessage, 4)
	go func() {
		defer close(messages)
		for {
			var message realtimeMessage
			if err := wsjson.Read(ctx, connection, &message); err != nil {
				return
			}
			messages <- message
		}
	}()
	if ready := <-messages; ready.Type != "ready" {
		t.Fatalf("unexpected first message: %#v", ready)
	}
	time.Sleep(200 * time.Millisecond)
	select {
	case <-ended:
		t.Fatal("the server dropped a responsive client")
	default:
	}
	stream.append(targetEvent(1))
	if event := <-messages; event.Version != 1 {
		t.Fatalf("expected event 1, got %#v", event)
	}
}

func TestRealtimeDropsAClientThatStopsAnsweringPings(t *testing.T) {
	url, ended := pingedStream(t, runEventHub(t, &fakeEventStream{}))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Un client qui ne lit jamais ne traite pas les pings : il ne répond pas.
	connection, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer connection.CloseNow()

	select {
	case <-ended:
	case <-ctx.Done():
		t.Fatal("the server kept a client that never answered its pings")
	}
}

func TestRealtimeClosesStreamsWhenTheHubStops(t *testing.T) {
	stream := &fakeEventStream{}
	hub := newEventHub(stream)
	hubCtx, stopHub := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		hub.Run(hubCtx)
	}()
	t.Cleanup(func() {
		stopHub()
		<-stopped
	})
	awaitEventHub(t, hub)
	connection, readCtx := dialRealtime(t, hub, "")
	readRealtime(t, readCtx, connection)

	stopHub()
	var message realtimeMessage
	err := wsjson.Read(readCtx, connection, &message)
	if status := websocket.CloseStatus(err); status != websocket.StatusGoingAway {
		t.Fatalf("expected a going-away close, got %v (%v)", status, err)
	}
}

// Derrière un relais qui réécrit Host vers l'adresse interne, le navigateur
// présente toujours l'origine publique. Le flux doit l'accepter, comme l'API
// REST, et refuser toute autre origine.
func TestRealtimeAcceptsThePublicOriginBehindAHostRewritingProxy(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	const publicOrigin = "http://cairnops.example.net"
	server := NewServer(ServerOptions{PublicURL: publicOrigin, Identity: &fakeIdentity{}, Events: runEventHub(t, &fakeEventStream{})})
	serveErrors := make(chan error, 1)
	go func() { serveErrors <- server.Serve(listener) }()
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-serveErrors
	})

	dial := func(origin string) (*websocket.Conn, *http.Response, error) {
		headers := http.Header{}
		headers.Set("Origin", origin)
		headers.Set("Cookie", (&http.Cookie{Name: "cairnops_session", Value: testSessionToken}).String())
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		t.Cleanup(cancel)
		return websocket.Dial(ctx, "ws://"+listener.Addr().String()+"/api/v1/events", &websocket.DialOptions{HTTPHeader: headers})
	}

	connection, response, err := dial(publicOrigin)
	if err != nil {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("the public origin must open the stream despite a rewritten Host: %v (status %d)", err, status)
	}
	defer connection.CloseNow()
	readCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if ready := readRealtime(t, readCtx, connection); ready.Type != "ready" {
		t.Fatalf("unexpected first message: %#v", ready)
	}

	if _, response, err := dial("http://intruder.example.net"); err == nil || response == nil || response.StatusCode != http.StatusForbidden {
		status := 0
		if response != nil {
			status = response.StatusCode
		}
		t.Fatalf("a foreign origin must be refused with 403, got status %d (%v)", status, err)
	}
}

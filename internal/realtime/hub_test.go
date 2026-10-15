package realtime

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"
)

type fakeSource struct {
	mu         sync.Mutex
	events     []Event
	reads      int
	failLatest int
	failReads  int
	// gate, s'il est posé, retient chaque lecture jusqu'à ce qu'on y écrive.
	gate chan struct{}
}

func (source *fakeSource) OldestVersion(context.Context) (int64, error) {
	return 0, nil
}

func (source *fakeSource) LatestVersion(context.Context) (int64, error) {
	source.mu.Lock()
	defer source.mu.Unlock()
	if source.failLatest > 0 {
		source.failLatest--
		return 0, errors.New("database unavailable")
	}
	if len(source.events) == 0 {
		return 0, nil
	}
	return source.events[len(source.events)-1].Version, nil
}

func (source *fakeSource) ListAfter(ctx context.Context, after int64, limit int) ([]Event, error) {
	source.mu.Lock()
	gate := source.gate
	source.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	source.mu.Lock()
	defer source.mu.Unlock()
	source.reads++
	if source.failReads > 0 {
		source.failReads--
		return nil, errors.New("database unavailable")
	}
	events := make([]Event, 0, limit)
	for _, event := range source.events {
		if event.Version > after {
			events = append(events, event)
			if len(events) == limit {
				break
			}
		}
	}
	return events, nil
}

func (source *fakeSource) append(count int) int64 {
	source.mu.Lock()
	defer source.mu.Unlock()
	var version int64
	for range count {
		version = int64(len(source.events) + 1)
		source.events = append(source.events, Event{Version: version, Kind: "target.changed", EntityType: "target", EntityID: strconv.FormatInt(version, 10)})
	}
	return version
}

func (source *fakeSource) readCount() int {
	source.mu.Lock()
	defer source.mu.Unlock()
	return source.reads
}

// manualListener laisse le test décider quand l'écoute s'établit, quelles
// notifications arrivent et quand la connexion tombe.
type manualListener struct {
	sessions chan *listenSession
}

type listenSession struct {
	notify func(string)
	drop   chan error
}

func newManualListener() *manualListener {
	return &manualListener{sessions: make(chan *listenSession, 8)}
}

func (listener *manualListener) Listen(ctx context.Context, listening func(), notify func(string)) error {
	session := &listenSession{notify: notify, drop: make(chan error, 1)}
	listening()
	listener.sessions <- session
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-session.drop:
		return err
	}
}

func (listener *manualListener) next(t *testing.T) *listenSession {
	t.Helper()
	select {
	case session := <-listener.sessions:
		return session
	case <-time.After(2 * time.Second):
		t.Fatal("the hub did not establish a LISTEN session")
		return nil
	}
}

func startHub(t *testing.T, source Source, listener Listener, options HubOptions) *Hub {
	t.Helper()
	if options.FallbackInterval == 0 {
		options.FallbackInterval = time.Hour
	}
	if options.RetryDelay == 0 {
		options.RetryDelay = 5 * time.Millisecond
	}
	hub := NewHub(source, listener, options)
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
	return hub
}

func subscribe(t *testing.T, hub *Hub) *Subscription {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		subscription, err := hub.Subscribe()
		if err == nil {
			t.Cleanup(subscription.Close)
			return subscription
		}
		if !errors.Is(err, ErrUnavailable) || time.Now().After(deadline) {
			t.Fatalf("subscribe: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
}

func receive(t *testing.T, subscription *Subscription) Event {
	t.Helper()
	select {
	case event, open := <-subscription.Events():
		if !open {
			t.Fatalf("subscription ended: %v", subscription.Err())
		}
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("no event delivered")
		return Event{}
	}
}

func expectNothing(t *testing.T, subscription *Subscription) {
	t.Helper()
	select {
	case event := <-subscription.Events():
		t.Fatalf("unexpected event %#v", event)
	case <-time.After(30 * time.Millisecond):
	}
}

func expectEnded(t *testing.T, subscription *Subscription, want error) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, open := <-subscription.Events():
			if !open {
				if err := subscription.Err(); !errors.Is(err, want) {
					t.Fatalf("expected %v, got %v", want, err)
				}
				return
			}
		case <-deadline:
			t.Fatalf("subscription did not end with %v", want)
		}
	}
}

func TestHubDeliversEachNotifiedEventToEverySubscriber(t *testing.T) {
	t.Parallel()
	source := &fakeSource{}
	source.append(3)
	listener := newManualListener()
	hub := startHub(t, source, listener, HubOptions{})
	session := listener.next(t)

	first, second := subscribe(t, hub), subscribe(t, hub)
	if first.Version() != 3 || second.Version() != 3 {
		t.Fatalf("subscriptions start from %d and %d, want 3", first.Version(), second.Version())
	}
	version := source.append(1)
	session.notify(strconv.FormatInt(version, 10))

	for _, subscription := range []*Subscription{first, second} {
		if event := receive(t, subscription); event.Version != 4 {
			t.Fatalf("expected version 4, got %#v", event)
		}
		expectNothing(t, subscription)
	}
}

func TestHubCoalescesNotificationsIntoOneRead(t *testing.T) {
	t.Parallel()
	gate := make(chan struct{})
	source := &fakeSource{}
	listener := newManualListener()
	hub := startHub(t, source, listener, HubOptions{})
	session := listener.next(t)
	subscription := subscribe(t, hub)

	// La lecture déclenchée par l'établissement de l'écoute est retenue : les
	// notifications qui arrivent pendant ce temps ne doivent en ajouter qu'une.
	source.mu.Lock()
	source.gate = gate
	source.mu.Unlock()
	session.notify("1")
	for range 50 {
		session.notify(strconv.FormatInt(source.append(1), 10))
	}
	close(gate)

	for want := int64(1); want <= 50; want++ {
		if event := receive(t, subscription); event.Version != want {
			t.Fatalf("expected version %d, got %#v", want, event)
		}
	}
	expectNothing(t, subscription)
	if reads := source.readCount(); reads > 3 {
		t.Fatalf("50 notifications caused %d reads, want at most 3", reads)
	}
}

func TestHubIgnoresNotificationsItHasAlreadyRead(t *testing.T) {
	t.Parallel()
	source := &fakeSource{}
	source.append(5)
	listener := newManualListener()
	hub := startHub(t, source, listener, HubOptions{})
	session := listener.next(t)
	subscribe(t, hub)
	waitFor(t, func() bool { return source.readCount() >= 1 })
	reads := source.readCount()

	session.notify("5")
	session.notify("2")
	time.Sleep(30 * time.Millisecond)
	if got := source.readCount(); got != reads {
		t.Fatalf("stale notifications caused %d reads", got-reads)
	}
}

func TestHubDisconnectsALaggingSubscriberWithoutHoldingOthers(t *testing.T) {
	t.Parallel()
	source := &fakeSource{}
	listener := newManualListener()
	hub := startHub(t, source, listener, HubOptions{BufferSize: 4})
	session := listener.next(t)
	lagging, reading := subscribe(t, hub), subscribe(t, hub)

	for version := int64(1); version <= 10; version++ {
		source.append(1)
		session.notify(strconv.FormatInt(version, 10))
		if event := receive(t, reading); event.Version != version {
			t.Fatalf("the reading subscriber expected %d, got %#v", version, event)
		}
	}
	expectEnded(t, lagging, ErrLagging)
}

func TestHubCatchesUpAfterTheListenerReconnects(t *testing.T) {
	t.Parallel()
	source := &fakeSource{}
	listener := newManualListener()
	hub := startHub(t, source, listener, HubOptions{})
	session := listener.next(t)
	subscription := subscribe(t, hub)

	waitFor(t, func() bool { return source.readCount() >= 1 })
	// Ces événements ne sont jamais notifiés, comme ceux enregistrés pendant une
	// interruption : le rétablissement de l'écoute doit suffire à les lire.
	source.append(2)
	session.drop <- errors.New("connection reset")
	listener.next(t)

	for want := int64(1); want <= 2; want++ {
		if event := receive(t, subscription); event.Version != want {
			t.Fatalf("expected version %d, got %#v", want, event)
		}
	}
}

func TestHubFallbackPollFindsEventsWithoutNotification(t *testing.T) {
	t.Parallel()
	source := &fakeSource{}
	listener := newManualListener()
	hub := startHub(t, source, listener, HubOptions{FallbackInterval: 20 * time.Millisecond})
	listener.next(t)
	subscription := subscribe(t, hub)

	source.append(1)
	if event := receive(t, subscription); event.Version != 1 {
		t.Fatalf("expected version 1, got %#v", event)
	}
}

func TestHubRetriesAFailedRead(t *testing.T) {
	t.Parallel()
	source := &fakeSource{}
	listener := newManualListener()
	hub := startHub(t, source, listener, HubOptions{})
	session := listener.next(t)
	subscription := subscribe(t, hub)
	waitFor(t, func() bool { return source.readCount() >= 1 })

	source.mu.Lock()
	source.failReads = 2
	source.mu.Unlock()
	session.notify(strconv.FormatInt(source.append(1), 10))
	if event := receive(t, subscription); event.Version != 1 {
		t.Fatalf("expected version 1, got %#v", event)
	}
}

func TestHubRefusesSubscriptionsUntilItKnowsTheCurrentVersion(t *testing.T) {
	t.Parallel()
	source := &fakeSource{failLatest: 3}
	source.append(2)
	hub := NewHub(source, newManualListener(), HubOptions{RetryDelay: 5 * time.Millisecond})
	if _, err := hub.Subscribe(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable before Run, got %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		hub.Run(ctx)
	}()
	subscription := subscribe(t, hub)
	if subscription.Version() != 2 {
		t.Fatalf("expected the subscription to start from 2, got %d", subscription.Version())
	}

	cancel()
	<-done
	expectEnded(t, subscription, ErrClosed)
	if _, err := hub.Subscribe(); !errors.Is(err, ErrClosed) {
		t.Fatalf("expected ErrClosed after Run, got %v", err)
	}
}

func TestSubscriptionCloseStopsDelivery(t *testing.T) {
	t.Parallel()
	source := &fakeSource{}
	listener := newManualListener()
	hub := startHub(t, source, listener, HubOptions{})
	session := listener.next(t)
	closed, open := subscribe(t, hub), subscribe(t, hub)

	closed.Close()
	closed.Close()
	session.notify(strconv.FormatInt(source.append(1), 10))
	if event := receive(t, open); event.Version != 1 {
		t.Fatalf("expected version 1, got %#v", event)
	}
	if _, ok := <-closed.Events(); ok {
		t.Fatal("a closed subscription still received an event")
	}
	if err := closed.Err(); err != nil {
		t.Fatalf("a subscription closed by its reader reports %v", err)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached")
		}
		time.Sleep(time.Millisecond)
	}
}

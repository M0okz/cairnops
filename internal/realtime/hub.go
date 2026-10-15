package realtime

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// Le hub lit la Suite d'événements une seule fois pour tout le processus et en
// distribue chaque événement aux connexions abonnées. PostgreSQL annonce chaque
// ajout par NOTIFY : le hub ne lit qu'à ce signal, et un sondage lent rattrape
// ce qu'une connexion LISTEN interrompue aurait manqué.

var (
	// ErrUnavailable signale un hub qui ne connaît pas encore la version
	// courante : un abonnement ne saurait pas d'où partir.
	ErrUnavailable = errors.New("event hub unavailable")
	// ErrLagging met fin à l'abonnement d'un lecteur trop lent, pour qu'il ne
	// retienne pas les autres. Il reprend depuis sa dernière version.
	ErrLagging = errors.New("event subscriber lagging")
	// ErrClosed met fin aux abonnements quand le hub s'arrête.
	ErrClosed = errors.New("event hub closed")
)

const (
	defaultBatchSize        = 250
	defaultBufferSize       = 512
	defaultFallbackInterval = 5 * time.Second
	defaultRetryDelay       = time.Second
	defaultMaxRetryDelay    = 30 * time.Second
	readTimeout             = 5 * time.Second
)

// Source est la Suite d'événements persistée.
type Source interface {
	LatestVersion(context.Context) (int64, error)
	OldestVersion(context.Context) (int64, error)
	ListAfter(context.Context, int64, int) ([]Event, error)
}

// Listener tient une session LISTEN jusqu'à la fin de ctx ou la perte de la
// connexion. Il appelle listening une fois l'écoute effective, puis notify à
// chaque notification reçue, avec sa charge utile.
type Listener interface {
	Listen(ctx context.Context, listening func(), notify func(payload string)) error
}

type HubOptions struct {
	BatchSize        int
	BufferSize       int
	FallbackInterval time.Duration
	RetryDelay       time.Duration
	MaxRetryDelay    time.Duration
	Logger           *slog.Logger
}

type Hub struct {
	source   Source
	listener Listener
	options  HubOptions
	logger   *slog.Logger

	wake    chan struct{}
	version atomic.Int64

	mu          sync.Mutex
	ready       bool
	closed      bool
	subscribers map[*Subscription]struct{}
}

func NewHub(source Source, listener Listener, options HubOptions) *Hub {
	if options.BatchSize <= 0 {
		options.BatchSize = defaultBatchSize
	}
	if options.BufferSize <= 0 {
		options.BufferSize = defaultBufferSize
	}
	if options.FallbackInterval <= 0 {
		options.FallbackInterval = defaultFallbackInterval
	}
	if options.RetryDelay <= 0 {
		options.RetryDelay = defaultRetryDelay
	}
	if options.MaxRetryDelay < options.RetryDelay {
		options.MaxRetryDelay = max(defaultMaxRetryDelay, options.RetryDelay)
	}
	logger := options.Logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Hub{
		source:      source,
		listener:    listener,
		options:     options,
		logger:      logger,
		wake:        make(chan struct{}, 1),
		subscribers: make(map[*Subscription]struct{}),
	}
}

// ListAfter lit la Suite directement : un client qui reprend après une
// interruption rejoue d'abord ce qu'il a manqué, sans passer par la diffusion.
func (hub *Hub) ListAfter(ctx context.Context, after int64, limit int) ([]Event, error) {
	return hub.source.ListAfter(ctx, after, limit)
}

// LatestVersion et OldestVersion bornent la fenêtre de reprise que la
// rétention laisse encore rejouer ; elles se lisent dans la Suite elle-même.
func (hub *Hub) LatestVersion(ctx context.Context) (int64, error) {
	return hub.source.LatestVersion(ctx)
}

func (hub *Hub) OldestVersion(ctx context.Context) (int64, error) {
	return hub.source.OldestVersion(ctx)
}

// Run fixe la version de départ, puis lit et diffuse jusqu'à la fin de ctx.
// Il ferme alors tous les abonnements avec ErrClosed.
func (hub *Hub) Run(ctx context.Context) {
	defer hub.shutdown()
	if !hub.initialize(ctx) {
		return
	}

	var listening sync.WaitGroup
	listening.Go(func() { hub.listen(ctx) })
	defer listening.Wait()

	fallback := time.NewTicker(hub.options.FallbackInterval)
	defer fallback.Stop()
	var retry <-chan time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-hub.wake:
		case <-fallback.C:
		case <-retry:
		}
		retry = nil
		if err := hub.catchUp(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			hub.logger.Error("read realtime events", "after", hub.version.Load(), "error", err)
			retry = time.After(hub.options.RetryDelay)
		}
	}
}

// Subscribe inscrit un lecteur. Il recevra, dans l'ordre, chaque événement que
// le hub lira après la version renvoyée par Subscription.Version.
func (hub *Hub) Subscribe() (*Subscription, error) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	if hub.closed {
		return nil, ErrClosed
	}
	if !hub.ready {
		return nil, ErrUnavailable
	}
	subscription := &Subscription{
		hub:     hub,
		version: hub.version.Load(),
		events:  make(chan Event, hub.options.BufferSize),
	}
	hub.subscribers[subscription] = struct{}{}
	return subscription, nil
}

func (hub *Hub) initialize(ctx context.Context) bool {
	delay := hub.options.RetryDelay
	for {
		readCtx, cancel := context.WithTimeout(ctx, readTimeout)
		version, err := hub.source.LatestVersion(readCtx)
		cancel()
		if err == nil {
			hub.version.Store(version)
			hub.mu.Lock()
			hub.ready = true
			hub.mu.Unlock()
			return true
		}
		if ctx.Err() != nil {
			return false
		}
		hub.logger.Error("read realtime cursor", "error", err)
		if !sleep(ctx, delay) {
			return false
		}
		delay = min(delay*2, hub.options.MaxRetryDelay)
	}
}

// listen garde une session LISTEN ouverte et la rétablit après chaque perte.
// Chaque rétablissement réveille le hub, qui rattrape ce qui a été ajouté
// pendant l'interruption depuis sa dernière version lue.
func (hub *Hub) listen(ctx context.Context) {
	delay := hub.options.RetryDelay
	for {
		established := false
		err := hub.listener.Listen(ctx, func() {
			established = true
			hub.signal()
		}, hub.notified)
		if ctx.Err() != nil {
			return
		}
		if established {
			delay = hub.options.RetryDelay
		}
		hub.logger.Warn("realtime listener interrupted", "error", err, "retry_in", delay.String())
		if !sleep(ctx, delay) {
			return
		}
		delay = min(delay*2, hub.options.MaxRetryDelay)
	}
}

// notified ignore une version déjà lue : la relire ne trouverait rien de plus.
// Une charge utile illisible réveille le hub par prudence.
func (hub *Hub) notified(payload string) {
	if version, err := strconv.ParseInt(payload, 10, 64); err == nil && version <= hub.version.Load() {
		return
	}
	hub.signal()
}

// signal coalesce les réveils : tant qu'une lecture est attendue, les
// notifications suivantes n'en ajoutent pas d'autre.
func (hub *Hub) signal() {
	select {
	case hub.wake <- struct{}{}:
	default:
	}
}

func (hub *Hub) catchUp(ctx context.Context) error {
	for {
		readCtx, cancel := context.WithTimeout(ctx, readTimeout)
		events, err := hub.source.ListAfter(readCtx, hub.version.Load(), hub.options.BatchSize)
		cancel()
		if err != nil {
			return err
		}
		if len(events) > 0 {
			hub.broadcast(events)
		}
		if len(events) < hub.options.BatchSize {
			return nil
		}
	}
}

// broadcast avance la version sous le même verrou que l'inscription : un
// abonné reçoit exactement les événements postérieurs à sa version de départ.
func (hub *Hub) broadcast(events []Event) {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	for subscription := range hub.subscribers {
	delivery:
		for _, event := range events {
			select {
			case subscription.events <- event:
			default:
				hub.endLocked(subscription, ErrLagging)
				break delivery
			}
		}
	}
	hub.version.Store(events[len(events)-1].Version)
}

func (hub *Hub) shutdown() {
	hub.mu.Lock()
	defer hub.mu.Unlock()
	hub.closed = true
	for subscription := range hub.subscribers {
		hub.endLocked(subscription, ErrClosed)
	}
}

func (hub *Hub) endLocked(subscription *Subscription, err error) {
	if _, ok := hub.subscribers[subscription]; !ok {
		return
	}
	delete(hub.subscribers, subscription)
	subscription.err = err
	close(subscription.events)
}

// Subscription est l'inscription d'un lecteur auprès du hub. Son canal se ferme
// quand le hub y met fin ; Err en donne alors la raison.
type Subscription struct {
	hub     *Hub
	version int64
	events  chan Event
	err     error
}

// Version est la dernière version lue par le hub au moment de l'inscription.
func (subscription *Subscription) Version() int64 {
	return subscription.version
}

func (subscription *Subscription) Events() <-chan Event {
	return subscription.events
}

// Err vaut ErrLagging ou ErrClosed une fois le canal fermé par le hub, nil
// sinon.
func (subscription *Subscription) Err() error {
	subscription.hub.mu.Lock()
	defer subscription.hub.mu.Unlock()
	return subscription.err
}

// Close désinscrit le lecteur. Il peut être appelé plusieurs fois.
func (subscription *Subscription) Close() {
	subscription.hub.mu.Lock()
	defer subscription.hub.mu.Unlock()
	if _, ok := subscription.hub.subscribers[subscription]; !ok {
		return
	}
	delete(subscription.hub.subscribers, subscription)
	close(subscription.events)
}

func sleep(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

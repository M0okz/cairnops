package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/M0okz/cairnops/internal/realtime"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	eventBatchSize    = 250
	eventWriteTimeout = 5 * time.Second
	eventPingInterval = 30 * time.Second
	eventPongTimeout  = 10 * time.Second
)

// EventStream diffuse la Suite d'événements : un abonnement reçoit ce qui est
// enregistré après sa version de départ, ListAfter rejoue ce qu'un client a
// manqué pendant une interruption.
type EventStream interface {
	Subscribe() (*realtime.Subscription, error)
	LatestVersion(context.Context) (int64, error)
	OldestVersion(context.Context) (int64, error)
	ListAfter(context.Context, int64, int) ([]realtime.Event, error)
}

type realtimeHandler struct {
	events       EventStream
	logger       *slog.Logger
	pingInterval time.Duration
	pongTimeout  time.Duration
}

type realtimeMessage struct {
	Type       string    `json:"type"`
	Version    int64     `json:"version"`
	Kind       string    `json:"kind,omitempty"`
	EntityType string    `json:"entity_type,omitempty"`
	EntityID   string    `json:"entity_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at,omitempty"`
	// Resync signale, sur le message ready, que le curseur demandé ne peut
	// plus être repris : le client recharge une projection complète.
	Resync bool `json:"resync,omitempty"`
}

func (handler realtimeHandler) stream(w http.ResponseWriter, r *http.Request) {
	after, resumes, err := requestedEventVersion(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// L'abonnement précède le rejeu : un événement enregistré pendant le rejeu
	// arrive par la diffusion, et le doublon éventuel est écarté par version.
	subscription, err := handler.events.Subscribe()
	if err != nil {
		handler.logger.Error("subscribe to realtime events", "error", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "event stream unavailable", "code": "event_stream_unavailable"})
		return
	}
	defer subscription.Close()
	resync := false
	if resumes {
		lookupCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		resumes, err = handler.resumable(lookupCtx, after)
		cancel()
		if err != nil {
			handler.logger.Error("read event horizon", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "event stream unavailable", "code": "event_stream_unavailable"})
			return
		}
		resync = !resumes
	}
	if !resumes {
		after = subscription.Version()
	}

	connection, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		handler.logger.Warn("accept realtime connection", "error", err)
		return
	}
	defer connection.CloseNow()

	streamCtx, cancel := context.WithCancel(r.Context())
	streamCtx = connection.CloseRead(streamCtx)

	var pinging sync.WaitGroup
	pinging.Go(func() { handler.ping(streamCtx, cancel, connection) })
	defer pinging.Wait()
	defer cancel()

	if err := writeRealtime(streamCtx, connection, realtimeMessage{Type: "ready", Version: after, Resync: resync}); err != nil {
		return
	}
	if resumes {
		next, ok := handler.sendPending(streamCtx, connection, after)
		if !ok {
			return
		}
		after = next
	}

	for {
		select {
		case <-streamCtx.Done():
			_ = connection.Close(websocket.StatusNormalClosure, "")
			return
		case event, open := <-subscription.Events():
			if !open {
				handler.closeEnded(connection, subscription.Err())
				return
			}
			if event.Version <= after {
				continue
			}
			if !handler.sendEvent(streamCtx, connection, event) {
				return
			}
			after = event.Version
		}
	}
}

// closeEnded explique au client pourquoi la diffusion s'arrête. Dans les deux
// cas, il se reconnecte avec son dernier curseur et ne perd rien.
func (handler realtimeHandler) closeEnded(connection *websocket.Conn, reason error) {
	switch {
	case errors.Is(reason, realtime.ErrLagging):
		handler.logger.Warn("disconnect lagging realtime subscriber")
		_ = connection.Close(websocket.StatusTryAgainLater, "event stream lagging")
	default:
		_ = connection.Close(websocket.StatusGoingAway, "server shutting down")
	}
}

// ping vérifie que le client répond encore, et entretient la connexion à
// travers les relais qui ferment les liaisons silencieuses.
func (handler realtimeHandler) ping(ctx context.Context, cancel context.CancelFunc, connection *websocket.Conn) {
	interval, timeout := handler.pingInterval, handler.pongTimeout
	if interval <= 0 {
		interval = eventPingInterval
	}
	if timeout <= 0 {
		timeout = eventPongTimeout
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, cancelPing := context.WithTimeout(ctx, timeout)
			err := connection.Ping(pingCtx)
			cancelPing()
			if err != nil {
				if ctx.Err() == nil {
					handler.logger.Debug("realtime client stopped answering pings", "error", err)
					cancel()
					connection.CloseNow()
				}
				return
			}
		}
	}
}

// resumable dit si les événements postérieurs au curseur sont tous encore
// conservés. Un curseur antérieur aux plus anciens a perdu des événements à la
// rétention ; un curseur postérieur au plus récent vient d'une autre base,
// restaurée depuis. Dans les deux cas, rejouer la suite tairait des
// changements : le client doit recharger une projection complète (ADR 0022).
func (handler realtimeHandler) resumable(ctx context.Context, after int64) (bool, error) {
	oldest, err := handler.events.OldestVersion(ctx)
	if err != nil {
		return false, err
	}
	latest, err := handler.events.LatestVersion(ctx)
	if err != nil {
		return false, err
	}
	return after <= latest && (oldest == 0 || after >= oldest-1), nil
}

func (handler realtimeHandler) sendPending(ctx context.Context, connection *websocket.Conn, after int64) (int64, bool) {
	for {
		events, err := handler.events.ListAfter(ctx, after, eventBatchSize)
		if err != nil {
			if ctx.Err() == nil {
				handler.logger.Error("read realtime events", "after", after, "error", err)
				_ = connection.Close(websocket.StatusInternalError, "event stream unavailable")
			}
			return after, false
		}
		for _, event := range events {
			if !handler.sendEvent(ctx, connection, event) {
				return after, false
			}
			after = event.Version
		}
		if len(events) < eventBatchSize {
			return after, true
		}
	}
}

func (handler realtimeHandler) sendEvent(ctx context.Context, connection *websocket.Conn, event realtime.Event) bool {
	message := realtimeMessage{
		Type: "event", Version: event.Version, Kind: event.Kind,
		EntityType: event.EntityType, EntityID: event.EntityID, OccurredAt: event.OccurredAt,
	}
	if err := writeRealtime(ctx, connection, message); err != nil {
		if ctx.Err() == nil && websocket.CloseStatus(err) == -1 {
			handler.logger.Debug("write realtime event", "error", err)
		}
		return false
	}
	return true
}

func writeRealtime(ctx context.Context, connection *websocket.Conn, message realtimeMessage) error {
	writeCtx, cancel := context.WithTimeout(ctx, eventWriteTimeout)
	defer cancel()
	return wsjson.Write(writeCtx, connection, message)
}

func requestedEventVersion(r *http.Request) (int64, bool, error) {
	raw := r.URL.Query().Get("after")
	if raw == "" {
		return 0, false, nil
	}
	version, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || version < 0 {
		return 0, false, errors.New("after must be a non-negative event version")
	}
	return version, true, nil
}

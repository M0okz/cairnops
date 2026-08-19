package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/M0okz/cairnops/internal/realtime"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const (
	eventBatchSize    = 250
	eventPollInterval = 500 * time.Millisecond
	eventWriteTimeout = 5 * time.Second
)

type EventStream interface {
	LatestVersion(context.Context) (int64, error)
	ListAfter(context.Context, int64, int) ([]realtime.Event, error)
}

type realtimeHandler struct {
	events EventStream
	logger *slog.Logger
}

type realtimeMessage struct {
	Type       string    `json:"type"`
	Version    int64     `json:"version"`
	Kind       string    `json:"kind,omitempty"`
	EntityType string    `json:"entity_type,omitempty"`
	EntityID   string    `json:"entity_id,omitempty"`
	OccurredAt time.Time `json:"occurred_at,omitempty"`
}

func (handler realtimeHandler) stream(w http.ResponseWriter, r *http.Request) {
	after, resumes, err := requestedEventVersion(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !resumes {
		lookupCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		after, err = handler.events.LatestVersion(lookupCtx)
		cancel()
		if err != nil {
			handler.logger.Error("read event cursor", "error", err)
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "event stream unavailable"})
			return
		}
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
	defer cancel()
	streamCtx = connection.CloseRead(streamCtx)

	if err := writeRealtime(streamCtx, connection, realtimeMessage{Type: "ready", Version: after}); err != nil {
		return
	}
	if next, ok := handler.sendPending(streamCtx, connection, after); ok {
		after = next
	} else {
		return
	}

	ticker := time.NewTicker(eventPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-streamCtx.Done():
			_ = connection.Close(websocket.StatusNormalClosure, "")
			return
		case <-ticker.C:
			next, ok := handler.sendPending(streamCtx, connection, after)
			if !ok {
				return
			}
			after = next
		}
	}
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
			message := realtimeMessage{
				Type: "event", Version: event.Version, Kind: event.Kind,
				EntityType: event.EntityType, EntityID: event.EntityID, OccurredAt: event.OccurredAt,
			}
			if err := writeRealtime(ctx, connection, message); err != nil {
				if ctx.Err() == nil && websocket.CloseStatus(err) == -1 {
					handler.logger.Debug("write realtime event", "error", err)
				}
				return after, false
			}
			after = event.Version
		}
		if len(events) < eventBatchSize {
			return after, true
		}
	}
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

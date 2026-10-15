package httpapi

import (
	"context"
	"log/slog"
	"strconv"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/realtime"
	"github.com/M0okz/cairnops/internal/retention"
	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/coder/websocket/wsjson"
)

// Un client qui revient après la fenêtre de reprise ne reçoit pas une suite
// amputée : la rétention a effacé ce qu'il a manqué, il recharge donc une
// projection complète, puis suit le flux depuis la version courante.
func TestRealtimeResumeAfterRetentionFallsBackToFullProjection(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	events := realtime.NewStore(pool)
	appendEvent := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, `SELECT cairnops_append_event('component.heartbeat', 'component', 'worker')`); err != nil {
			t.Fatal(err)
		}
	}

	appendEvent()
	cursor, err := events.LatestVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		appendEvent()
	}
	if _, err := pool.Exec(ctx, `UPDATE cairnops_events SET occurred_at = now() - interval '3 days'`); err != nil {
		t.Fatal(err)
	}
	appendEvent()
	purger := retention.NewPurger(pool, retention.DefaultPolicy(), slog.New(slog.DiscardHandler))
	if _, err := purger.Purge(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	latest, err := events.LatestVersion(ctx)
	if err != nil {
		t.Fatal(err)
	}

	hub := realtime.NewHub(events, realtime.NewPostgresListener(pool), realtime.HubOptions{})
	hubCtx, stopHub := context.WithCancel(ctx)
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

	connection, readCtx := dialRealtime(t, hub, "?after="+strconv.FormatInt(cursor, 10))
	var ready realtimeMessage
	if err := wsjson.Read(readCtx, connection, &ready); err != nil {
		t.Fatalf("read ready message: %v", err)
	}
	if ready.Type != "ready" || !ready.Resync || ready.Version != latest {
		t.Fatalf("a purged cursor must ask for a full projection at version %d, got %#v", latest, ready)
	}

	appendEvent()
	var event realtimeMessage
	if err := wsjson.Read(readCtx, connection, &event); err != nil {
		t.Fatalf("read event message: %v", err)
	}
	if event.Type != "event" || event.Version <= latest {
		t.Fatalf("the stream must continue after the current version, got %#v", event)
	}

	// Un curseur encore couvert par la fenêtre se reprend sans rechargement.
	resumed, resumedCtx := dialRealtime(t, hub, "?after="+strconv.FormatInt(latest, 10))
	var resumedReady realtimeMessage
	if err := wsjson.Read(resumedCtx, resumed, &resumedReady); err != nil {
		t.Fatalf("read ready message: %v", err)
	}
	if resumedReady.Type != "ready" || resumedReady.Resync || resumedReady.Version != latest {
		t.Fatalf("a retained cursor must resume, got %#v", resumedReady)
	}
}

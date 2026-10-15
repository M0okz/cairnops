package realtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Channel est le canal sur lequel cairnops_append_event annonce chaque version.
const Channel = "cairnops_events"

// Une connexion LISTEN silencieuse peut être morte sans le savoir : passé ce
// délai sans notification, l'écouteur la sonde.
const listenerProbeInterval = 30 * time.Second

// PostgresListener écoute sur une connexion dédiée, hors du pool : elle reste
// occupée en permanence et ne doit pas priver les requêtes d'une connexion.
type PostgresListener struct {
	config *pgx.ConnConfig
}

// ListenerApplicationName désigne la connexion d'écoute dans pg_stat_activity.
const ListenerApplicationName = "cairnops-realtime-listener"

func NewPostgresListener(pool *pgxpool.Pool) *PostgresListener {
	config := pool.Config().ConnConfig.Copy()
	config.RuntimeParams["application_name"] = ListenerApplicationName
	return &PostgresListener{config: config}
}

func (listener *PostgresListener) Listen(ctx context.Context, listening func(), notify func(string)) error {
	connectCtx, cancel := context.WithTimeout(ctx, readTimeout)
	connection, err := pgx.ConnectConfig(connectCtx, listener.config.Copy())
	cancel()
	if err != nil {
		return fmt.Errorf("connect realtime listener: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = connection.Close(closeCtx)
	}()

	if _, err := connection.Exec(ctx, "LISTEN "+Channel); err != nil {
		return fmt.Errorf("listen on %s: %w", Channel, err)
	}
	listening()

	for {
		waitCtx, cancel := context.WithTimeout(ctx, listenerProbeInterval)
		notification, err := connection.WaitForNotification(waitCtx)
		cancel()
		switch {
		case err == nil:
			notify(notification.Payload)
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(err, context.DeadlineExceeded):
			probeCtx, cancel := context.WithTimeout(ctx, readTimeout)
			err := connection.Ping(probeCtx)
			cancel()
			if err != nil {
				return fmt.Errorf("probe realtime listener: %w", err)
			}
		default:
			return fmt.Errorf("wait for realtime notification: %w", err)
		}
	}
}

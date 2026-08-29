package notifications_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/notifications"
	"github.com/M0okz/cairnops/internal/testsupport"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Le Canal intégré est posé par la migration : chaque instance en a un, et un
// seul. Ces scénarios s'appuient dessus plutôt que d'en créer un.
func inAppChannel(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		SELECT id::text FROM cairnops_notification_channels WHERE kind = 'in_app'
	`).Scan(&id); err != nil {
		t.Fatalf("le Canal intégré est absent de l'instance : %v", err)
	}
	return id
}

func seedAccount(t *testing.T, pool *pgxpool.Pool, role string) string {
	t.Helper()
	var id string
	username := fmt.Sprintf("%s-%d", role, time.Now().UTC().UnixNano())
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO cairnops_users (username, display_name, password_hash, role)
		VALUES ($1, 'Compte de test', 'not-used', $2) RETURNING id::text
	`, username, role).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func seedActiveIncident(t *testing.T, pool *pgxpool.Pool, severity string) (targetID, incidentID string) {
	t.Helper()
	ctx := context.Background()
	name := fmt.Sprintf("Cible intégrée %d", time.Now().UTC().UnixNano())
	if err := pool.QueryRow(ctx, `INSERT INTO cairnops_targets (name) VALUES ($1) RETURNING id::text`, name).Scan(&targetID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO cairnops_incidents (
			target_id, nature_key, nature_label, status, source_severity, effective_severity, opened_at
		) VALUES ($1::uuid, 'native:http', 'Indisponibilité', 'active', $2, $2, now())
		RETURNING id::text
	`, targetID, severity).Scan(&incidentID); err != nil {
		t.Fatal(err)
	}
	return targetID, incidentID
}

// Le trajet complet : une ouverture atteint tout le monde, la Résolution ne
// retrouve que ceux qui ont eu l'ouverture, et lire ne concerne que soi.
func TestPostgresInAppDeliveryReachesEveryActiveAccount(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := notifications.NewPostgresStore(pool)

	operator := seedAccount(t, pool, "operator")
	observer := seedAccount(t, pool, "observer")
	_, incidentID := seedActiveIncident(t, pool, "major")

	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	firing, err := store.Claim(ctx, "worker-inbox")
	if err != nil {
		t.Fatal(err)
	}
	if firing.ChannelKind != notifications.KindInApp {
		t.Fatalf("la livraison réclamée n'est pas celle du Canal intégré : %+v", firing)
	}

	delivered, err := store.Deliver(ctx, firing)
	if err != nil {
		t.Fatal(err)
	}
	if delivered < 2 {
		t.Fatalf("l'ouverture n'a pas atteint les deux comptes : %d", delivered)
	}
	if err := store.Complete(ctx, firing.ID, "worker-inbox"); err != nil {
		t.Fatal(err)
	}

	// Chacun lit la sienne, et personne ne lit celle d'un autre.
	for _, userID := range []string{operator, observer} {
		inbox, err := store.Inbox(ctx, userID, 0)
		if err != nil {
			t.Fatal(err)
		}
		if inbox.Unread != 1 || len(inbox.Entries) != 1 {
			t.Fatalf("boîte inattendue pour %s : %+v", userID, inbox)
		}
		if inbox.Entries[0].IncidentID != incidentID || inbox.Entries[0].EventKind != "firing" {
			t.Fatalf("entrée inattendue : %+v", inbox.Entries[0])
		}
		if inbox.Entries[0].NatureKey != "native:http" {
			t.Fatalf("la clé stable de Nature manque : %+v", inbox.Entries[0])
		}
		if inbox.Entries[0].ReadAt != nil {
			t.Fatalf("une entrée fraîche est déjà lue : %+v", inbox.Entries[0])
		}
	}

	// Lire est un geste personnel : il ne touche pas la boîte du voisin.
	read, err := store.MarkRead(ctx, operator, nil)
	if err != nil {
		t.Fatal(err)
	}
	if read != 1 {
		t.Fatalf("une seule entrée devait être marquée lue : %d", read)
	}
	operatorInbox, err := store.Inbox(ctx, operator, 0)
	if err != nil {
		t.Fatal(err)
	}
	observerInbox, err := store.Inbox(ctx, observer, 0)
	if err != nil {
		t.Fatal(err)
	}
	if operatorInbox.Unread != 0 || observerInbox.Unread != 1 {
		t.Fatalf("la lecture a débordé sur une autre boîte : %d / %d", operatorInbox.Unread, observerInbox.Unread)
	}
	if operatorInbox.Entries[0].ReadAt == nil {
		t.Fatal("l'entrée lue ne porte pas sa date de lecture")
	}
}

// Vider retire le bruit du volet, mais l'ouverture reste une mémoire de
// routage : sa Résolution doit encore revenir à la même personne.
func TestPostgresDismissedInboxStillRoutesTheResolution(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := notifications.NewPostgresStore(pool)

	actor := seedAccount(t, pool, "operator")
	neighbor := seedAccount(t, pool, "observer")
	targetID, incidentID := seedActiveIncident(t, pool, "major")
	for _, userID := range []string{actor, neighbor} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO cairnops_notification_inbox (
				user_id, incident_id, target_id, event_kind, target_name,
				nature_label, severity, occurred_at
			) VALUES ($1::uuid, $2::uuid, $3::uuid, 'firing', 'Cible intégrée',
				'Indisponibilité', 'major', now())
		`, userID, incidentID, targetID); err != nil {
			t.Fatal(err)
		}
	}

	dismissed, err := store.Dismiss(ctx, actor)
	if err != nil {
		t.Fatal(err)
	}
	if dismissed != 1 {
		t.Fatalf("une entrée devait quitter le volet : %d", dismissed)
	}
	actorInbox, err := store.Inbox(ctx, actor, 0)
	if err != nil {
		t.Fatal(err)
	}
	neighborInbox, err := store.Inbox(ctx, neighbor, 0)
	if err != nil {
		t.Fatal(err)
	}
	if actorInbox.Unread != 0 || len(actorInbox.Entries) != 0 {
		t.Fatalf("le volet vidé contient encore l'ouverture : %+v", actorInbox)
	}
	if neighborInbox.Unread != 1 || len(neighborInbox.Entries) != 1 {
		t.Fatalf("vider un volet a touché celui d'un autre compte : %+v", neighborInbox)
	}

	resolvedAt := time.Now().UTC()
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents
		SET status = 'resolved', resolved_at = $2
		WHERE id = $1::uuid
	`, incidentID, resolvedAt); err != nil {
		t.Fatal(err)
	}
	delivered, err := store.Deliver(ctx, notifications.Delivery{
		IncidentID: incidentID, ChannelID: inAppChannel(t, pool),
		ChannelKind: notifications.KindInApp, EventKind: "resolved",
		TargetName: "Cible intégrée", NatureLabel: "Indisponibilité",
		Severity: "major", OpenedAt: resolvedAt.Add(-time.Hour), ResolvedAt: &resolvedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if delivered != 2 {
		t.Fatalf("la Résolution n'a pas retrouvé les deux destinataires : %d", delivered)
	}
	actorInbox, err = store.Inbox(ctx, actor, 0)
	if err != nil {
		t.Fatal(err)
	}
	if actorInbox.Unread != 1 || len(actorInbox.Entries) != 1 || actorInbox.Entries[0].EventKind != "resolved" {
		t.Fatalf("la Résolution n'est pas revenue dans le volet vidé : %+v", actorInbox)
	}
}

func TestPostgresInAppResolutionReachesTheOpeningRecipientsOnly(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := notifications.NewPostgresStore(pool)

	early := seedAccount(t, pool, "operator")
	_, incidentID := seedActiveIncident(t, pool, "critical")

	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	firing, err := store.Claim(ctx, "worker-inbox")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Deliver(ctx, firing); err != nil {
		t.Fatal(err)
	}
	if err := store.Complete(ctx, firing.ID, "worker-inbox"); err != nil {
		t.Fatal(err)
	}

	// Quelqu'un arrive après l'ouverture : il n'a pas eu le début, il n'aura
	// pas la fin.
	late := seedAccount(t, pool, "observer")

	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents SET status = 'resolved', resolved_at = now() WHERE id = $1::uuid
	`, incidentID); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	resolved, err := store.Claim(ctx, "worker-inbox")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.EventKind != "resolved" {
		t.Fatalf("la livraison réclamée n'est pas la Résolution : %+v", resolved)
	}
	if _, err := store.Deliver(ctx, resolved); err != nil {
		t.Fatal(err)
	}

	earlyInbox, err := store.Inbox(ctx, early, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(earlyInbox.Entries) != 2 {
		t.Fatalf("le destinataire de l'ouverture n'a pas reçu la Résolution : %+v", earlyInbox.Entries)
	}
	lateInbox, err := store.Inbox(ctx, late, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(lateInbox.Entries) != 0 {
		t.Fatalf("un compte arrivé après l'ouverture a reçu la Résolution : %+v", lateInbox.Entries)
	}
}

// L'Acquittement arrête ce qui n'est pas encore parti — c'est la garde de la
// boîte d'envoi, et le Canal intégré en hérite sans rien ajouter.
func TestPostgresInAppOpeningIsCancelledByAcknowledgement(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := notifications.NewPostgresStore(pool)

	actor := seedAccount(t, pool, "operator")
	_, incidentID := seedActiveIncident(t, pool, "major")

	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_incidents SET acknowledged_at = now(), acknowledged_by = $2::uuid WHERE id = $1::uuid
	`, incidentID, actor); err != nil {
		t.Fatal(err)
	}
	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := store.Claim(ctx, "worker-inbox"); err == nil {
		t.Fatal("une ouverture acquittée est encore partie")
	}
	inbox, err := store.Inbox(ctx, actor, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox.Entries) != 0 {
		t.Fatalf("une notification est arrivée malgré l'Acquittement : %+v", inbox.Entries)
	}
}

// La Gravité gouverne le Canal intégré comme les autres : le Canal posé par la
// migration ne route pas l'Information.
func TestPostgresInAppHonoursTheChannelSeverities(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := notifications.NewPostgresStore(pool)

	actor := seedAccount(t, pool, "operator")
	seedActiveIncident(t, pool, "information")

	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(ctx, "worker-inbox"); err == nil {
		t.Fatal("un Incident d'Information a été routé alors que le Canal ne le demande pas")
	}
	inbox, err := store.Inbox(ctx, actor, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox.Entries) != 0 {
		t.Fatalf("boîte non vide : %+v", inbox.Entries)
	}
}

// Un compte désactivé ne reçoit plus rien : il n'a plus accès à l'instance, et
// lui déposer des nouvelles n'aurait pas de lecteur.
func TestPostgresInAppSkipsDeactivatedAccounts(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	store := notifications.NewPostgresStore(pool)

	retired := seedAccount(t, pool, "operator")
	if _, err := pool.Exec(ctx, `
		UPDATE cairnops_users SET deactivated_at = now() WHERE id = $1::uuid
	`, retired); err != nil {
		t.Fatal(err)
	}
	seedActiveIncident(t, pool, "critical")

	if err := store.Schedule(ctx); err != nil {
		t.Fatal(err)
	}
	firing, err := store.Claim(ctx, "worker-inbox")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Deliver(ctx, firing); err != nil {
		t.Fatal(err)
	}

	inbox, err := store.Inbox(ctx, retired, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbox.Entries) != 0 {
		t.Fatalf("un compte désactivé a reçu une notification : %+v", inbox.Entries)
	}
}

// Le Canal intégré est unique : c'est l'instance elle-même, pas une destination
// que l'on choisirait deux fois.
func TestPostgresInAppChannelIsUnique(t *testing.T) {
	ctx := context.Background()
	pool := testsupport.Pool(t)
	inAppChannel(t, pool)

	if _, err := pool.Exec(ctx, `
		INSERT INTO cairnops_notification_channels (
			kind, name, endpoint, credential_sealed, severities,
			enabled, status, encrypted_transport, last_checked_at, last_error
		) VALUES ('in_app', 'Doublon', '', '', ARRAY['critical']::text[], true, 'connected', true, now(), '')
	`); err == nil {
		t.Fatal("un second Canal intégré a été accepté")
	}
}

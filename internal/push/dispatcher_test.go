package push

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/M0okz/cairnops/internal/devices"
	"github.com/M0okz/cairnops/internal/secretbox"
	"golang.org/x/crypto/curve25519"
)

type dispatcherStore struct {
	delivery   Delivery
	claimed    bool
	completed  int64
	failed     int64
	disabled   int64
	configured bool
	statusErr  error
}

func (store *dispatcherStore) Claim(context.Context, string) (Delivery, error) {
	if store.claimed {
		return Delivery{}, ErrNoDelivery
	}
	store.claimed = true
	return store.delivery, nil
}
func (store *dispatcherStore) Complete(_ context.Context, id int64, _ string) error {
	store.completed = id
	return nil
}
func (store *dispatcherStore) Fail(_ context.Context, id int64, _, _ string) error {
	store.failed = id
	return nil
}
func (store *dispatcherStore) DisableDevice(_ context.Context, id int64, _, _ string) error {
	store.disabled = id
	return nil
}
func (store *dispatcherStore) SetRelayStatus(_ context.Context, configured bool, err error) error {
	store.configured, store.statusErr = configured, err
	return nil
}

type recordingRelay struct {
	delivery DeliveryRequest
	err      error
}

func (relay *recordingRelay) Ping(context.Context) error { return relay.err }
func (relay *recordingRelay) Deliver(_ context.Context, delivery DeliveryRequest) error {
	relay.delivery = delivery
	return relay.err
}

func TestDispatcherDeliversAnOpaqueEncryptedProjection(t *testing.T) {
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	sealedRecipient, err := box.Seal([]byte("opaque-recipient-0123456789"), devices.PushRecipientPurpose)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := make([]byte, curve25519.ScalarSize)
	if _, err := rand.Read(privateKey); err != nil {
		t.Fatal(err)
	}
	publicKey, err := curve25519.X25519(privateKey, curve25519.Basepoint)
	if err != nil {
		t.Fatal(err)
	}
	store := &dispatcherStore{delivery: Delivery{
		ID: 7, RecipientSealed: sealedRecipient, EncryptionPublicKey: publicKey,
		Locale: "fr", NotificationContent: "complete", EventKind: "firing",
		IncidentID: "incident-id", TargetName: "PostgreSQL", NatureLabel: "Indisponible",
		Severity: "critical",
	}}
	relay := &recordingRelay{}
	dispatcher := NewDispatcher(store, relay, box, "worker", "https://cairnops.example.test", nil)
	if err := dispatcher.Process(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.completed != 7 || store.failed != 0 {
		t.Fatalf("unexpected delivery result: completed=%d failed=%d", store.completed, store.failed)
	}
	if relay.delivery.Recipient != "opaque-recipient-0123456789" || relay.delivery.Envelope.Ciphertext == "" {
		t.Fatalf("relay did not receive the opaque envelope: %#v", relay.delivery)
	}
	if relay.delivery.Recipient == store.delivery.TargetName || relay.delivery.CollapseKey == store.delivery.IncidentID {
		t.Fatalf("relay metadata exposed operational identifiers: %#v", relay.delivery)
	}
	if relay.delivery.Priority != "high" {
		t.Fatalf("incident opening lost its alert priority: %#v", relay.delivery)
	}
}

func TestSilentBurstUpdateCollapsesOnThePersistentBurst(t *testing.T) {
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	sealedRecipient, err := box.Seal([]byte("opaque-recipient-burst-012345"), devices.PushRecipientPurpose)
	if err != nil {
		t.Fatal(err)
	}
	store := &dispatcherStore{delivery: Delivery{
		ID: 8, RecipientSealed: sealedRecipient,
		EncryptionPublicKey: curve25519.Basepoint,
		EventKind:           "firing", IncidentID: "incident-anchor", BurstID: "burst-persistent",
		Revision: 4, PresentationMode: "silent", Severity: "major",
	}}
	relay := &recordingRelay{}
	dispatcher := NewDispatcher(store, relay, box, "worker", "https://cairnops.example.test", nil)
	if err := dispatcher.Process(context.Background()); err != nil {
		t.Fatal(err)
	}
	if relay.delivery.CollapseKey != collapseKey("burst-persistent") || relay.delivery.Priority != "normal" {
		t.Fatalf("silent burst update did not collapse quietly: %#v", relay.delivery)
	}
}

func TestProbeReportsUnconfiguredRelayWithoutClaiming(t *testing.T) {
	store := &dispatcherStore{}
	dispatcher := NewDispatcher(store, nil, nil, "worker", "https://cairnops.example.test", nil)
	if err := dispatcher.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.configured || store.statusErr == nil {
		t.Fatalf("unconfigured relay was not reported: configured=%v err=%v", store.configured, store.statusErr)
	}
}

func TestDispatcherDisablesOnlyAnExpiredRecipient(t *testing.T) {
	box, err := secretbox.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	sealedRecipient, err := box.Seal([]byte("expired-recipient-0123456789"), devices.PushRecipientPurpose)
	if err != nil {
		t.Fatal(err)
	}
	store := &dispatcherStore{delivery: Delivery{
		ID: 9, RecipientSealed: sealedRecipient,
		EncryptionPublicKey: curve25519.Basepoint, EventKind: "firing", IncidentID: "incident",
	}}
	relay := &recordingRelay{err: &HTTPError{StatusCode: 410}}
	dispatcher := NewDispatcher(store, relay, box, "worker", "https://cairnops.example.test", nil)
	if err := dispatcher.Process(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.disabled != 9 || store.failed != 0 || store.statusErr != nil {
		t.Fatalf("expired recipient degraded the relay: disabled=%d failed=%d status=%v", store.disabled, store.failed, store.statusErr)
	}
}

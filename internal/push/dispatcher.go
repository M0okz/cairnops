package push

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/M0okz/cairnops/internal/devices"
	"github.com/M0okz/cairnops/internal/secretbox"
)

type Dispatcher struct {
	store         DeliveryStore
	relay         Relay
	secrets       *secretbox.Box
	workerID      string
	publicURL     string
	logger        *slog.Logger
	interval      time.Duration
	probeInterval time.Duration
	now           func() time.Time
}

func NewDispatcher(store DeliveryStore, relay Relay, secrets *secretbox.Box, workerID, publicURL string, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{
		store: store, relay: relay, secrets: secrets, workerID: workerID,
		publicURL: publicURL, logger: logger, interval: 2 * time.Second,
		probeInterval: 30 * time.Second, now: time.Now,
	}
}

func (dispatcher *Dispatcher) Run(ctx context.Context) error {
	if err := dispatcher.Probe(ctx); err != nil {
		return err
	}
	deliveryTicker := time.NewTicker(dispatcher.interval)
	probeTicker := time.NewTicker(dispatcher.probeInterval)
	defer deliveryTicker.Stop()
	defer probeTicker.Stop()
	for {
		if err := dispatcher.Process(ctx); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-probeTicker.C:
			if err := dispatcher.Probe(ctx); err != nil {
				return err
			}
		case <-deliveryTicker.C:
		}
	}
}

func (dispatcher *Dispatcher) Probe(ctx context.Context) error {
	if dispatcher.relay == nil {
		return dispatcher.store.SetRelayStatus(ctx, false, errors.New("push relay is not configured"))
	}
	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	err := dispatcher.relay.Ping(probeCtx)
	if err != nil {
		dispatcher.logger.Warn("push relay unavailable", "error", err)
	}
	return dispatcher.store.SetRelayStatus(ctx, true, err)
}

func (dispatcher *Dispatcher) Process(ctx context.Context) error {
	if dispatcher.relay == nil {
		return nil
	}
	for processed := 0; processed < 20; processed++ {
		delivery, err := dispatcher.store.Claim(ctx, dispatcher.workerID)
		if errors.Is(err, ErrNoDelivery) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("claim push notification: %w", err)
		}
		if err := dispatcher.deliver(ctx, delivery); err != nil {
			if RecipientExpired(err) {
				if disableErr := dispatcher.store.DisableDevice(ctx, delivery.ID, dispatcher.workerID, err.Error()); disableErr != nil {
					return disableErr
				}
				continue
			}
			if failErr := dispatcher.store.Fail(ctx, delivery.ID, dispatcher.workerID, err.Error()); failErr != nil {
				return failErr
			}
			if statusErr := dispatcher.store.SetRelayStatus(ctx, true, err); statusErr != nil {
				return statusErr
			}
			continue
		}
		if err := dispatcher.store.Complete(ctx, delivery.ID, dispatcher.workerID); err != nil {
			return err
		}
		if err := dispatcher.store.SetRelayStatus(ctx, true, nil); err != nil {
			return err
		}
	}
	return nil
}

func (dispatcher *Dispatcher) deliver(ctx context.Context, delivery Delivery) error {
	recipient, err := dispatcher.secrets.Open(delivery.RecipientSealed, devices.PushRecipientPurpose)
	if err != nil {
		return fmt.Errorf("open push recipient: %w", err)
	}
	envelope, err := Encrypt(delivery.EncryptionPublicKey, messageFor(delivery, dispatcher.publicURL))
	if err != nil {
		return err
	}
	priority := "normal"
	if delivery.PresentationMode == "alert" || (delivery.PresentationMode == "" && delivery.EventKind == "firing") {
		priority = "high"
	}
	return dispatcher.relay.Deliver(ctx, DeliveryRequest{
		Recipient: string(recipient), Envelope: envelope,
		CollapseKey: collapseKey(delivery.IncidentID), Priority: priority,
		ExpiresAt: dispatcher.now().UTC().Add(time.Hour),
	})
}

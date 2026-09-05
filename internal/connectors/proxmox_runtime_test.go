package connectors

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/connectors/proxmox"
	"github.com/M0okz/cairnops/internal/incidents"
	"github.com/M0okz/cairnops/internal/secretbox"
)

type proxmoxMeasurementFailure struct{ runtimeStore }

func (s *proxmoxMeasurementFailure) RefreshProxmox(_ context.Context, connector RuntimeConnector, _ string, _ []proxmox.Resource) ([]RuntimeBinding, error) {
	return connector.Bindings, nil
}
func (s *proxmoxMeasurementFailure) RecordIntegrationObservations(context.Context, time.Time, []IntegrationObservation) error {
	return errors.New("measurement storage unavailable")
}

type proxmoxEvidenceCapture struct{ snapshot incidents.EvidenceSnapshot }

func (s *proxmoxEvidenceCapture) ApplyEvidenceSnapshot(_ context.Context, snapshot incidents.EvidenceSnapshot) error {
	s.snapshot = snapshot
	return nil
}

func TestProxmoxMeasurementFailureDoesNotFailVerifiedProductCycle(t *testing.T) {
	box, _ := secretbox.New(bytes.Repeat([]byte{8}, 32))
	raw, _ := json.Marshal(proxmox.Credentials{TokenID: "qa@pve!test", Secret: "qa"})
	sealed, _ := box.Seal(raw, "connector:proxmox:https://pve.example.net")
	store := &proxmoxMeasurementFailure{runtimeStore: runtimeStore{connectors: []RuntimeConnector{{ID: "connector", Endpoint: "https://pve.example.net", CredentialSealed: sealed, Bindings: []RuntimeBinding{{ID: "binding", ExternalID: "node/alpha", TargetID: "target"}}}}}}
	evidence := &proxmoxEvidenceCapture{}
	client := &proxmoxInventory{resources: []proxmox.Resource{{ID: "node/alpha", Type: "node", Node: "alpha", Name: "alpha", Status: "offline"}}}
	if err := NewProxmoxSynchronizer(store, evidence, client, box, "owner", nil).tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !store.completed || store.failed != "" || len(evidence.snapshot.Facts) != 1 {
		t.Fatal("a measurement gap changed the verified product result")
	}
}

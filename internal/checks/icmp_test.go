package checks

import (
	"context"
	"testing"
)

func TestResolveICMPTargetRejectsInvalidFamily(t *testing.T) {
	t.Parallel()

	if _, _, _, _, _, err := resolveICMPTarget(context.Background(), "localhost", "ipx"); err == nil {
		t.Fatal("expected invalid family to be rejected")
	}
}

func TestResolveICMPTargetFindsIPv4Localhost(t *testing.T) {
	t.Parallel()

	ip, network, protocol, _, _, err := resolveICMPTarget(context.Background(), "127.0.0.1", "ipv4")
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "127.0.0.1" || network != "udp4" || protocol != 1 {
		t.Fatalf("unexpected resolution: %s %s %d", ip, network, protocol)
	}
}

package push

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestRelayClientSendsOnlyOpaqueDeliveryEnvelope(t *testing.T) {
	var received DeliveryRequest
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/deliveries" || r.Method != http.MethodPost {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatal(err)
		}
		return &http.Response{
			StatusCode: http.StatusAccepted,
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Header:     make(http.Header),
		}, nil
	})}

	request := DeliveryRequest{
		Recipient: "opaque-recipient", Envelope: Envelope{Version: 1, Ciphertext: "sealed"},
		CollapseKey: "opaque-collapse", Priority: "high", ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	if err := NewRelayClient("https://relay.example.test", client).Deliver(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if received.Recipient != request.Recipient || received.Envelope.Ciphertext != "sealed" {
		t.Fatalf("unexpected relay projection: %#v", received)
	}
}

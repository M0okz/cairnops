package pushrelay

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/push"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (function roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestAPNSProviderSendsEncryptedAlertWithTokenAuthentication(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	var received *http.Request
	var payload map[string]any
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		received = request
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	})}
	provider := newAPNSProvider(
		"https://api.push.test", "https://api.sandbox.test",
		"fr.cairnops.ios", "KEYID12345", "TEAMID1234", privateKey, client,
	)
	provider.now = func() time.Time { return now }
	delivery := validDelivery(now)
	if err := provider.Deliver(context.Background(), Registration{Platform: "ios", Environment: "production", DeviceToken: "0011aabb"}, delivery); err != nil {
		t.Fatal(err)
	}
	if received.URL.String() != "https://api.push.test/3/device/0011aabb" {
		t.Fatalf("unexpected APNs URL: %s", received.URL)
	}
	if received.Header.Get("apns-topic") != "fr.cairnops.ios" ||
		received.Header.Get("apns-push-type") != "alert" ||
		received.Header.Get("apns-priority") != "10" {
		t.Fatalf("unexpected APNs headers: %#v", received.Header)
	}
	authorization := strings.TrimPrefix(received.Header.Get("Authorization"), "bearer ")
	parts := strings.Split(authorization, ".")
	if len(parts) != 3 {
		t.Fatalf("invalid provider token: %q", authorization)
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !rawECDSASignatureValid(&privateKey.PublicKey, parts[0]+"."+parts[1], signature) {
		t.Fatal("provider token signature is invalid")
	}
	aps, ok := payload["aps"].(map[string]any)
	if !ok || aps["mutable-content"] != float64(1) {
		t.Fatalf("APNs payload does not invoke the notification service: %#v", payload)
	}
	cairnops, ok := payload["cairnops"].(map[string]any)
	if !ok || cairnops["envelope"] == nil {
		t.Fatalf("APNs payload lost the encrypted envelope: %#v", payload)
	}
}

func TestAPNSProviderSendsSilentUpdateAsBackgroundNotification(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var received *http.Request
	var payload map[string]any
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		received = request
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
	})}
	provider := newAPNSProvider(
		"https://api.push.test", "https://api.sandbox.test",
		"fr.cairnops.ios", "KEYID12345", "TEAMID1234", privateKey, client,
	)
	delivery := validDelivery(time.Now().UTC())
	delivery.Priority = "normal"
	if err := provider.Deliver(context.Background(), Registration{Platform: "ios", Environment: "production", DeviceToken: "0011aabb"}, delivery); err != nil {
		t.Fatal(err)
	}
	if received.Header.Get("apns-push-type") != "background" || received.Header.Get("apns-priority") != "5" {
		t.Fatalf("silent update used alert APNs headers: %#v", received.Header)
	}
	aps, ok := payload["aps"].(map[string]any)
	if !ok || aps["content-available"] != float64(1) {
		t.Fatalf("silent update is not background-capable: %#v", payload)
	}
	for _, visibleKey := range []string{"alert", "sound", "mutable-content"} {
		if _, exists := aps[visibleKey]; exists {
			t.Fatalf("silent update still contains visible key %q: %#v", visibleKey, payload)
		}
	}
	if cairnops, ok := payload["cairnops"].(map[string]any); !ok || cairnops["envelope"] == nil {
		t.Fatalf("silent update lost the encrypted envelope: %#v", payload)
	}
}

func TestAPNSProviderClassifiesExpiredDeviceToken(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(`{"reason":"BadDeviceToken"}`)),
			Header:     make(http.Header),
		}, nil
	})}
	provider := newAPNSProvider(
		"https://api.push.test", "https://api.sandbox.test",
		"fr.cairnops.ios", "KEYID12345", "TEAMID1234", privateKey, client,
	)
	err = provider.Deliver(context.Background(), Registration{Platform: "ios", Environment: "sandbox", DeviceToken: "0011"}, validDelivery(time.Now().UTC()))
	failure := providerError(err)
	if failure == nil || !failure.RecipientExpired() {
		t.Fatalf("APNs expiry was not classified: %v", err)
	}
}

func TestAPNSProviderRoutesDevelopmentRegistrationsToSandbox(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var receivedURL string
	client := &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		receivedURL = request.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(nil)),
			Header:     make(http.Header),
		}, nil
	})}
	provider := newAPNSProvider(
		"https://api.push.test", "https://api.sandbox.test",
		"fr.cairnops.ios", "KEYID12345", "TEAMID1234", privateKey, client,
	)
	registration := Registration{Platform: "ios", Environment: "sandbox", DeviceToken: "0011aabb"}
	if err := provider.Deliver(context.Background(), registration, validDelivery(time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	if receivedURL != "https://api.sandbox.test/3/device/0011aabb" {
		t.Fatalf("development registration used %s", receivedURL)
	}
}

func validDelivery(now time.Time) push.DeliveryRequest {
	recipient := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	return push.DeliveryRequest{
		Recipient: recipient,
		Envelope: push.Envelope{
			Version: 1, EphemeralPublicKey: "ephemeral", Nonce: "nonce", Ciphertext: "ciphertext",
		},
		CollapseKey: "opaque-collapse", Priority: "high", ExpiresAt: now.Add(time.Hour),
	}
}

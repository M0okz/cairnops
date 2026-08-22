package pushrelay

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/M0okz/cairnops/internal/push"
)

type recordingProvider struct {
	delivery     push.DeliveryRequest
	registration Registration
	err          error
}

func (provider *recordingProvider) Deliver(_ context.Context, registration Registration, delivery push.DeliveryRequest) error {
	provider.registration = registration
	provider.delivery = delivery
	return provider.err
}

func TestHandlerRegistersRotatesAndDeliversWithoutExposingAPNSToken(t *testing.T) {
	store := testFileStore(t, t.TempDir())
	provider := &recordingProvider{}
	handler := NewHandler(store, provider, slog.New(slog.NewTextHandler(io.Discard, nil)))

	registrationBody := []byte(`{"platform":"ios","environment":"sandbox","device_token":"0011aabb"}`)
	register := httptest.NewRequest(http.MethodPost, "/v1/registrations", bytes.NewReader(registrationBody))
	register.Header.Set("Content-Type", "application/json")
	registerResponse := httptest.NewRecorder()
	handler.ServeHTTP(registerResponse, register)
	if registerResponse.Code != http.StatusCreated {
		t.Fatalf("registration returned %d: %s", registerResponse.Code, registerResponse.Body.String())
	}
	var credentials RegistrationCredentials
	if err := json.NewDecoder(registerResponse.Body).Decode(&credentials); err != nil {
		t.Fatal(err)
	}

	delivery := validDelivery(time.Now().UTC())
	delivery.Recipient = credentials.Recipient
	deliveryBody, err := json.Marshal(delivery)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/deliveries", bytes.NewReader(deliveryBody))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("delivery returned %d: %s", response.Code, response.Body.String())
	}
	if provider.registration.DeviceToken != "0011aabb" || provider.registration.Environment != "sandbox" ||
		provider.delivery.Recipient != credentials.Recipient {
		t.Fatalf("relay did not resolve the opaque recipient: %#v %#v", provider.registration, provider.delivery)
	}

	rotate := httptest.NewRequest(http.MethodPut, "/v1/registrations/"+credentials.Recipient, bytes.NewReader([]byte(`{"platform":"ios","environment":"production","device_token":"ffee"}`)))
	rotate.Header.Set("Content-Type", "application/json")
	rotate.Header.Set("Authorization", "Bearer "+credentials.ManagementToken)
	rotateResponse := httptest.NewRecorder()
	handler.ServeHTTP(rotateResponse, rotate)
	if rotateResponse.Code != http.StatusNoContent {
		t.Fatalf("rotation returned %d: %s", rotateResponse.Code, rotateResponse.Body.String())
	}
	rotated, err := store.Resolve(credentials.Recipient)
	if err != nil || rotated.DeviceToken != "ffee" || rotated.Environment != "production" {
		t.Fatalf("rotation was not persisted: %#v %v", rotated, err)
	}
}

func TestHandlerExpiresRecipientRejectedByAPNS(t *testing.T) {
	store := testFileStore(t, t.TempDir())
	credentials, err := store.Register("ios", "production", "0011")
	if err != nil {
		t.Fatal(err)
	}
	provider := &recordingProvider{err: &ProviderError{StatusCode: http.StatusGone, Reason: "Unregistered"}}
	handler := NewHandler(store, provider, slog.New(slog.NewTextHandler(io.Discard, nil)))
	delivery := validDelivery(time.Now().UTC())
	delivery.Recipient = credentials.Recipient
	body, err := json.Marshal(delivery)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/deliveries", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusGone {
		t.Fatalf("expired recipient returned %d: %s", response.Code, response.Body.String())
	}
	if _, err := store.Resolve(credentials.Recipient); err != ErrRegistrationNotFound {
		t.Fatalf("expired recipient remained registered: %v", err)
	}
}

package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/M0okz/cairnops/internal/devices"
	identitymodel "github.com/M0okz/cairnops/internal/identity"
)

type fakeDevices struct {
	authenticated devices.AuthenticatedDevice
	claimInput    devices.ClaimInput
	createFor     string
	revoked       string
}

func (fake *fakeDevices) CreatePairing(_ context.Context, userID string) (devices.Invitation, error) {
	fake.createFor = userID
	return devices.Invitation{Pairing: devices.Pairing{ID: "10000000-0000-0000-0000-000000000001"}, Token: "one-time"}, nil
}
func (*fakeDevices) GetPairing(context.Context, string, string) (devices.Pairing, error) {
	return devices.Pairing{}, nil
}
func (fake *fakeDevices) ClaimPairing(_ context.Context, _ string, input devices.ClaimInput) (devices.PairingResult, error) {
	fake.claimInput = input
	return devices.PairingResult{Status: "awaiting_confirmation"}, nil
}
func (*fakeDevices) ConfirmPairing(context.Context, string, string) (devices.Pairing, error) {
	return devices.Pairing{Status: "confirmed"}, nil
}
func (*fakeDevices) PairingResult(context.Context, string) (devices.PairingResult, error) {
	return devices.PairingResult{Status: "confirmed", DeviceToken: "device-secret"}, nil
}
func (*fakeDevices) CancelPairing(context.Context, string, string) error { return nil }
func (fake *fakeDevices) Authenticate(_ context.Context, token string) (devices.AuthenticatedDevice, error) {
	if token != "device-token" {
		return devices.AuthenticatedDevice{}, devices.ErrInvalidDevice
	}
	return fake.authenticated, nil
}
func (*fakeDevices) List(context.Context, identitymodel.Principal) ([]devices.Device, error) {
	return []devices.Device{}, nil
}
func (*fakeDevices) Update(context.Context, identitymodel.Principal, string, devices.UpdateInput) (devices.Device, error) {
	return devices.Device{}, nil
}
func (fake *fakeDevices) Revoke(_ context.Context, _ identitymodel.Principal, deviceID string) error {
	fake.revoked = deviceID
	return nil
}
func (fake *fakeDevices) RevokeSelf(_ context.Context, deviceID string) error {
	fake.revoked = deviceID
	return nil
}

func TestDeviceBearerAuthenticatesExistingAPI(t *testing.T) {
	t.Parallel()
	fake := &fakeDevices{authenticated: devices.AuthenticatedDevice{
		Principal: identitymodel.Principal{ID: "user-id", Role: "observer"},
		DeviceID:  "20000000-0000-0000-0000-000000000002",
	}}
	server := NewServer(ServerOptions{
		Identity: &fakeIdentity{}, Devices: fake,
		SystemHealth: fakeSystemHealth{},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/system/health", nil)
	request.Header.Set("Authorization", "Bearer device-token")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("device bearer did not authenticate: %d %s", response.Code, response.Body)
	}
}

func TestPairingCreationRequiresBrowserSession(t *testing.T) {
	t.Parallel()
	fake := &fakeDevices{authenticated: devices.AuthenticatedDevice{
		Principal: identitymodel.Principal{ID: "user-id", Role: "administrator"}, DeviceID: "device-id",
	}}
	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Devices: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/device-pairings", nil)
	request.Header.Set("Authorization", "Bearer device-token")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized || fake.createFor != "" {
		t.Fatalf("a mobile identity created a pairing: %d for=%q", response.Code, fake.createFor)
	}
}

func TestPairingClaimUsesOneTimeBearer(t *testing.T) {
	t.Parallel()
	fake := &fakeDevices{}
	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Devices: fake})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/device-pairings/claim", strings.NewReader(`{
		"name":"iPhone", "platform":"ios", "encryption_public_key":"key", "push_recipient":"recipient"
	}`))
	request.Header.Set("Authorization", "Bearer pairing-token")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || fake.claimInput.Name != "iPhone" {
		t.Fatalf("pairing claim did not reach the service: %d %s", response.Code, response.Body)
	}
}

func TestDeviceLogoutRevokesOnlyCurrentDevice(t *testing.T) {
	t.Parallel()
	deviceID := "20000000-0000-0000-0000-000000000002"
	fake := &fakeDevices{authenticated: devices.AuthenticatedDevice{
		Principal: identitymodel.Principal{ID: "user-id", Role: "operator"}, DeviceID: deviceID,
	}}
	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Devices: fake})
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/session", nil)
	request.Header.Set("Authorization", "Bearer device-token")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || fake.revoked != deviceID {
		t.Fatalf("device logout did not revoke its identity: %d revoked=%q", response.Code, fake.revoked)
	}
}

func TestDeviceBearerCannotRevokeSiblingDevice(t *testing.T) {
	t.Parallel()
	fake := &fakeDevices{authenticated: devices.AuthenticatedDevice{
		Principal: identitymodel.Principal{ID: "user-id", Role: "administrator"},
		DeviceID:  "20000000-0000-0000-0000-000000000002",
	}}
	server := NewServer(ServerOptions{Identity: &fakeIdentity{}, Devices: fake})
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/30000000-0000-0000-0000-000000000003", nil)
	request.Header.Set("Authorization", "Bearer device-token")
	response := httptest.NewRecorder()
	server.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || fake.revoked != "" {
		t.Fatalf("device bearer revoked a sibling: %d revoked=%q", response.Code, fake.revoked)
	}
}

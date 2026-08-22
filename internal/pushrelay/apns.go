package pushrelay

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/M0okz/cairnops/internal/push"
)

const (
	APNSPayloadLimit   = 4096
	providerTokenReuse = 50 * time.Minute
)

type ProviderError struct {
	StatusCode int
	Reason     string
}

func (err *ProviderError) Error() string {
	if err.Reason == "" {
		return fmt.Sprintf("APNs returned HTTP %d", err.StatusCode)
	}
	return fmt.Sprintf("APNs returned %s (HTTP %d)", err.Reason, err.StatusCode)
}

func (err *ProviderError) RecipientExpired() bool {
	return err.StatusCode == http.StatusGone ||
		err.Reason == "BadDeviceToken" || err.Reason == "DeviceTokenNotForTopic" ||
		err.Reason == "Unregistered"
}

func (err *ProviderError) Temporary() bool {
	return err.StatusCode == http.StatusTooManyRequests || err.StatusCode >= 500
}

type Provider interface {
	Deliver(context.Context, Registration, push.DeliveryRequest) error
}

type APNSProvider struct {
	client             *http.Client
	productionEndpoint string
	sandboxEndpoint    string
	topic              string
	keyID              string
	teamID             string
	privateKey         *ecdsa.PrivateKey
	now                func() time.Time
	tokenMu            sync.Mutex
	cachedToken        string
	tokenIssued        time.Time
}

func NewAPNSProvider(topic, keyID, teamID, keyFile string, client *http.Client) (*APNSProvider, error) {
	keyPEM, err := os.ReadFile(strings.TrimSpace(keyFile))
	if err != nil {
		return nil, fmt.Errorf("read APNs signing key: %w", err)
	}
	privateKey, err := parseAPNSPrivateKey(keyPEM)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return newAPNSProvider(
		"https://api.push.apple.com", "https://api.sandbox.push.apple.com",
		topic, keyID, teamID, privateKey, client,
	), nil
}

func newAPNSProvider(
	productionEndpoint, sandboxEndpoint, topic, keyID, teamID string,
	privateKey *ecdsa.PrivateKey, client *http.Client,
) *APNSProvider {
	return &APNSProvider{
		client:             client,
		productionEndpoint: strings.TrimSuffix(productionEndpoint, "/"),
		sandboxEndpoint:    strings.TrimSuffix(sandboxEndpoint, "/"),
		topic:              topic,
		keyID:              keyID, teamID: teamID, privateKey: privateKey, now: time.Now,
	}
}

func (provider *APNSProvider) Deliver(ctx context.Context, registration Registration, delivery push.DeliveryRequest) error {
	if registration.Platform != "ios" {
		return &ProviderError{StatusCode: http.StatusBadRequest, Reason: "UnsupportedPlatform"}
	}
	payload, err := makeAPNSPayload(delivery)
	if err != nil {
		return err
	}
	providerToken, err := provider.authorizationToken()
	if err != nil {
		return err
	}
	endpoint := provider.productionEndpoint
	if registration.Environment == "sandbox" {
		endpoint = provider.sandboxEndpoint
	} else if registration.Environment != "production" {
		return &ProviderError{StatusCode: http.StatusBadRequest, Reason: "InvalidEnvironment"}
	}
	requestURL := endpoint + "/3/device/" + url.PathEscape(registration.DeviceToken)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build APNs request: %w", err)
	}
	request.Header.Set("Authorization", "bearer "+providerToken)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("apns-topic", provider.topic)
	request.Header.Set("apns-push-type", "alert")
	request.Header.Set("apns-priority", apnsPriority(delivery.Priority))
	request.Header.Set("apns-expiration", strconv.FormatInt(delivery.ExpiresAt.UTC().Unix(), 10))
	request.Header.Set("apns-collapse-id", delivery.CollapseKey)
	if requestID, err := newRequestID(); err == nil {
		request.Header.Set("apns-id", requestID)
	}

	response, err := provider.client.Do(request)
	if err != nil {
		return fmt.Errorf("contact APNs: %w", err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode == http.StatusOK {
		return nil
	}
	var failure struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(body, &failure)
	return &ProviderError{StatusCode: response.StatusCode, Reason: failure.Reason}
}

func (provider *APNSProvider) authorizationToken() (string, error) {
	provider.tokenMu.Lock()
	defer provider.tokenMu.Unlock()
	now := provider.now().UTC()
	if provider.cachedToken != "" && now.Sub(provider.tokenIssued) < providerTokenReuse {
		return provider.cachedToken, nil
	}
	token, err := signProviderToken(provider.privateKey, provider.keyID, provider.teamID, now)
	if err != nil {
		return "", err
	}
	provider.cachedToken, provider.tokenIssued = token, now
	return token, nil
}

func parseAPNSPrivateKey(encoded []byte) (*ecdsa.PrivateKey, error) {
	block, rest := pem.Decode(encoded)
	if block == nil || block.Type != "PRIVATE KEY" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, fmt.Errorf("APNs signing key must be one PEM PKCS#8 private key")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse APNs signing key: %w", err)
	}
	privateKey, ok := key.(*ecdsa.PrivateKey)
	if !ok || privateKey.Curve != elliptic.P256() {
		return nil, fmt.Errorf("APNs signing key must use the P-256 curve")
	}
	return privateKey, nil
}

func signProviderToken(privateKey *ecdsa.PrivateKey, keyID, teamID string, now time.Time) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "ES256", "kid": keyID})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{"iss": teamID, "iat": now.Unix()})
	if err != nil {
		return "", err
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign APNs provider token: %w", err)
	}
	signature := make([]byte, 64)
	r.FillBytes(signature[:32])
	s.FillBytes(signature[32:])
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func makeAPNSPayload(delivery push.DeliveryRequest) ([]byte, error) {
	if !validRelayToken(delivery.Recipient) {
		return nil, &ProviderError{StatusCode: http.StatusBadRequest, Reason: "InvalidRecipient"}
	}
	if delivery.Envelope.Version != 1 || delivery.Envelope.EphemeralPublicKey == "" ||
		delivery.Envelope.Nonce == "" || delivery.Envelope.Ciphertext == "" {
		return nil, &ProviderError{StatusCode: http.StatusBadRequest, Reason: "InvalidEnvelope"}
	}
	if delivery.CollapseKey == "" || len(delivery.CollapseKey) > 64 {
		return nil, &ProviderError{StatusCode: http.StatusBadRequest, Reason: "InvalidCollapseKey"}
	}
	if delivery.ExpiresAt.IsZero() {
		return nil, &ProviderError{StatusCode: http.StatusBadRequest, Reason: "InvalidExpiration"}
	}
	payload := map[string]any{
		"aps": map[string]any{
			"alert": map[string]string{
				"title": "CairnOps",
				"body":  "Ouvrez CairnOps pour consulter la mise à jour.",
			},
			"mutable-content": 1,
			"sound":           "default",
			"category":        "CAIRNOPS_INCIDENT",
			"thread-id":       delivery.CollapseKey,
		},
		"cairnops": map[string]any{"envelope": delivery.Envelope},
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode APNs payload: %w", err)
	}
	if len(encoded) > APNSPayloadLimit {
		return nil, &ProviderError{StatusCode: http.StatusBadRequest, Reason: "PayloadTooLarge"}
	}
	return encoded, nil
}

func apnsPriority(priority string) string {
	if priority == "high" {
		return "10"
	}
	return "5"
}

func newRequestID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

func providerError(err error) *ProviderError {
	var result *ProviderError
	if errors.As(err, &result) {
		return result
	}
	return nil
}

func rawECDSASignatureValid(publicKey *ecdsa.PublicKey, unsigned string, signature []byte) bool {
	if len(signature) != 64 {
		return false
	}
	digest := sha256.Sum256([]byte(unsigned))
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	return ecdsa.Verify(publicKey, digest[:], r, s)
}

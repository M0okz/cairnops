package pushrelay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/M0okz/cairnops/internal/push"
)

const maximumRelayBody = 32 << 10

type RegistrationStore interface {
	Register(platform, environment, deviceToken string) (RegistrationCredentials, error)
	Rotate(recipient, managementToken, platform, environment, deviceToken string) error
	Resolve(recipient string) (Registration, error)
	Delete(recipient, managementToken string) error
	Expire(recipient string) error
	Ping() error
}

type Handler struct {
	store               RegistrationStore
	provider            Provider
	deliveryLimiter     *RateLimiter
	registrationLimiter *RateLimiter
	logger              *slog.Logger
	serveMux            *http.ServeMux
}

func NewHandler(store RegistrationStore, provider Provider, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	handler := &Handler{
		store: store, provider: provider,
		deliveryLimiter:     NewRateLimiter(60, time.Minute),
		registrationLimiter: NewRateLimiter(20, time.Hour),
		logger:              logger,
		serveMux:            http.NewServeMux(),
	}
	handler.serveMux.HandleFunc("GET /v1/health", handler.health)
	handler.serveMux.HandleFunc("POST /v1/registrations", handler.register)
	handler.serveMux.HandleFunc("PUT /v1/registrations/{recipient}", handler.rotate)
	handler.serveMux.HandleFunc("DELETE /v1/registrations/{recipient}", handler.remove)
	handler.serveMux.HandleFunc("POST /v1/deliveries", handler.deliver)
	return handler
}

func NewServer(address string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr: address, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
}

func (handler *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	handler.serveMux.ServeHTTP(w, r)
}

func (handler *Handler) health(w http.ResponseWriter, _ *http.Request) {
	if handler.store == nil || handler.provider == nil {
		writeRelayError(w, http.StatusServiceUnavailable, "relay is not ready")
		return
	}
	if err := handler.store.Ping(); err != nil {
		handler.logger.Error("push relay storage unavailable", "error", err)
		writeRelayError(w, http.StatusServiceUnavailable, "relay storage is unavailable")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type registrationRequest struct {
	Platform    string `json:"platform"`
	Environment string `json:"environment"`
	DeviceToken string `json:"device_token"`
}

func (handler *Handler) register(w http.ResponseWriter, r *http.Request) {
	if !handler.registrationLimiter.Allow(registrationRateKey(r)) {
		w.Header().Set("Retry-After", "3600")
		writeRelayError(w, http.StatusTooManyRequests, "push registration rate limit exceeded")
		return
	}
	var input registrationRequest
	if err := decodeRelayJSON(w, r, &input); err != nil {
		return
	}
	credentials, err := handler.store.Register(input.Platform, input.Environment, input.DeviceToken)
	if err != nil {
		writeRelayError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeRelayJSON(w, http.StatusCreated, credentials)
}

func (handler *Handler) rotate(w http.ResponseWriter, r *http.Request) {
	managementToken, ok := managementBearer(r)
	if !ok {
		unauthorizedManagement(w)
		return
	}
	var input registrationRequest
	if err := decodeRelayJSON(w, r, &input); err != nil {
		return
	}
	err := handler.store.Rotate(
		r.PathValue("recipient"), managementToken,
		input.Platform, input.Environment, input.DeviceToken,
	)
	switch {
	case errors.Is(err, ErrRegistrationNotFound):
		writeRelayError(w, http.StatusNotFound, "push registration not found")
	case errors.Is(err, ErrInvalidManagement):
		unauthorizedManagement(w)
	case err != nil:
		writeRelayError(w, http.StatusBadRequest, err.Error())
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (handler *Handler) remove(w http.ResponseWriter, r *http.Request) {
	managementToken, ok := managementBearer(r)
	if !ok {
		unauthorizedManagement(w)
		return
	}
	err := handler.store.Delete(r.PathValue("recipient"), managementToken)
	switch {
	case errors.Is(err, ErrRegistrationNotFound):
		writeRelayError(w, http.StatusNotFound, "push registration not found")
	case errors.Is(err, ErrInvalidManagement):
		unauthorizedManagement(w)
	case err != nil:
		handler.logger.Error("remove push registration", "error", err)
		writeRelayError(w, http.StatusInternalServerError, "internal relay error")
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (handler *Handler) deliver(w http.ResponseWriter, r *http.Request) {
	var delivery push.DeliveryRequest
	if err := decodeRelayJSON(w, r, &delivery); err != nil {
		return
	}
	if !validRelayToken(delivery.Recipient) {
		writeRelayError(w, http.StatusNotFound, "push recipient not found")
		return
	}
	limitKey := digestToken(delivery.Recipient)
	if !handler.deliveryLimiter.Allow(limitKey) {
		w.Header().Set("Retry-After", "60")
		writeRelayError(w, http.StatusTooManyRequests, "push recipient rate limit exceeded")
		return
	}
	registration, err := handler.store.Resolve(delivery.Recipient)
	if errors.Is(err, ErrRegistrationNotFound) {
		writeRelayError(w, http.StatusNotFound, "push recipient not found")
		return
	}
	if err != nil {
		handler.logger.Error("resolve push registration", "error", err)
		writeRelayError(w, http.StatusServiceUnavailable, "relay storage is unavailable")
		return
	}
	if err := handler.provider.Deliver(r.Context(), registration, delivery); err != nil {
		handler.writeProviderError(w, delivery.Recipient, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func registrationRateKey(r *http.Request) string {
	forwarded := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for index := len(forwarded) - 1; index >= 0; index-- {
		if address := strings.TrimSpace(forwarded[index]); address != "" {
			return digestToken(address)
		}
	}
	return digestToken(r.RemoteAddr)
}

func (handler *Handler) writeProviderError(w http.ResponseWriter, recipient string, err error) {
	providerFailure := providerError(err)
	if providerFailure == nil {
		handler.logger.Warn("push provider unavailable", "error", err)
		writeRelayError(w, http.StatusServiceUnavailable, "push provider is unavailable")
		return
	}
	if providerFailure.RecipientExpired() {
		if removeErr := handler.store.Expire(recipient); removeErr != nil && !errors.Is(removeErr, ErrRegistrationNotFound) {
			handler.logger.Error("expire push registration", "error", removeErr)
		}
		writeRelayError(w, http.StatusGone, "push recipient expired")
		return
	}
	switch {
	case providerFailure.StatusCode == http.StatusTooManyRequests:
		w.Header().Set("Retry-After", "60")
		writeRelayError(w, http.StatusTooManyRequests, "push provider rate limit exceeded")
	case providerFailure.Temporary():
		handler.logger.Warn("push provider temporarily unavailable", "reason", providerFailure.Reason)
		writeRelayError(w, http.StatusServiceUnavailable, "push provider is unavailable")
	case providerFailure.StatusCode == http.StatusBadRequest:
		writeRelayError(w, http.StatusBadRequest, "invalid push delivery")
	default:
		handler.logger.Error("push provider rejected relay configuration", "status", providerFailure.StatusCode, "reason", providerFailure.Reason)
		writeRelayError(w, http.StatusServiceUnavailable, "push provider rejected relay configuration")
	}
}

func decodeRelayJSON(w http.ResponseWriter, r *http.Request, target any) error {
	contentType := r.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || mediaType != "application/json" {
			writeRelayError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
			return fmt.Errorf("unsupported content type")
		}
	}
	reader := http.MaxBytesReader(w, r.Body, maximumRelayBody)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeRelayError(w, http.StatusBadRequest, "invalid JSON body")
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeRelayError(w, http.StatusBadRequest, "JSON body must contain one object")
		return fmt.Errorf("multiple JSON values")
	}
	return nil
}

func managementBearer(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return token, validRelayToken(token)
}

func unauthorizedManagement(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="cairnops-push-registration"`)
	writeRelayError(w, http.StatusUnauthorized, "push registration authentication required")
}

func writeRelayJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeRelayError(w http.ResponseWriter, status int, message string) {
	writeRelayJSON(w, status, map[string]string{"error": message})
}

type HealthPinger struct {
	URL    string
	Client *http.Client
}

func (pinger HealthPinger) Ping(ctx context.Context) error {
	client := pinger.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, pinger.URL, nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("relay health returned HTTP %d", response.StatusCode)
	}
	return nil
}

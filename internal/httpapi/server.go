package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/M0okz/cairnops/internal/reconciliation"
	"github.com/M0okz/cairnops/internal/version"
)

type Pinger interface {
	Ping(context.Context) error
}

type ServerOptions struct {
	Address         string
	WebDir          string
	PublicURL       string
	Pinger          Pinger
	Logger          *slog.Logger
	Service         string
	BootstrapToken  string
	Identity        Identity
	OIDC            OIDC
	ControlPlane    ControlPlane
	Metrics         Metrics
	Indicators      Indicators
	Connectors      Connectors
	Webhooks        Webhooks
	Incidents       Incidents
	Bursts          Bursts
	Maintenances    Maintenances
	Notifications   Notifications
	Devices         DeviceManager
	Events          EventStream
	SystemHealth    SystemHealth
	Reconciliations reconciliation.Service
}

func NewServer(options ServerOptions) *http.Server {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	service := options.Service
	if service == "" {
		service = "cairnops"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": service})
	})
	mux.HandleFunc("GET /api/v1/health/ready", readinessHandler(options.Pinger, service))
	mux.HandleFunc("GET /api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, version.Info())
	})
	var identityHTTP identityHandler
	if options.Identity != nil {
		identityHTTP = identityHandler{
			identity: options.Identity, devices: options.Devices,
			security: newSessionSecurity(options.PublicURL), logger: logger,
		}
		bootstrap := newAuthenticator(options.BootstrapToken)
		mux.HandleFunc("GET /api/v1/setup/status", identityHTTP.setupStatus)
		mux.Handle("POST /api/v1/setup", identityHTTP.requireSameOrigin(bootstrap.require(http.HandlerFunc(identityHTTP.initialize))))
		mux.Handle("PATCH /api/v1/instance", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(identityHTTP.renameInstance)))))
		mux.Handle("POST /api/v1/session", identityHTTP.requireSameOrigin(http.HandlerFunc(identityHTTP.login)))
		mux.Handle("GET /api/v1/session", identityHTTP.requireSession(http.HandlerFunc(identityHTTP.currentSession)))
		mux.Handle("DELETE /api/v1/session", identityHTTP.requireSameOrigin(identityHTTP.requireSession(http.HandlerFunc(identityHTTP.logout))))
		mux.Handle("PUT /api/v1/session/password", identityHTTP.requireSameOrigin(identityHTTP.requireSession(http.HandlerFunc(identityHTTP.changeOwnPassword))))
		mux.Handle("GET /api/v1/users", identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(identityHTTP.listAccounts))))
		mux.Handle("POST /api/v1/users", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireLocalAdministrator(http.HandlerFunc(identityHTTP.createAccount)))))
		mux.Handle("PATCH /api/v1/users/{userID}", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireLocalAdministrator(http.HandlerFunc(identityHTTP.updateAccount)))))
		mux.Handle("POST /api/v1/users/{userID}/password", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireLocalAdministrator(http.HandlerFunc(identityHTTP.setUserPassword)))))
		// Désactiver et réactiver empruntent la même route, comme la
		// suspension d'un Connecteur : c'est un état que l'on pose et retire.
		mux.Handle("POST /api/v1/users/{userID}/deactivation", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireLocalAdministrator(http.HandlerFunc(identityHTTP.deactivateAccount)))))
		mux.Handle("DELETE /api/v1/users/{userID}/deactivation", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireLocalAdministrator(http.HandlerFunc(identityHTTP.reactivateAccount)))))
		// Porte de secours : sans session, puisqu'elle sert quand plus personne
		// ne peut en ouvrir. Le Jeton d'amorçage tient lieu de preuve, comme
		// pour la mise en service.
		mux.Handle("POST /api/v1/recovery", identityHTTP.requireSameOrigin(bootstrap.require(http.HandlerFunc(identityHTTP.recoverPassword))))
		if options.OIDC != nil {
			oidcHTTP := oidcHandler{oidc: options.OIDC, identity: identityHTTP}
			mux.HandleFunc("GET /api/v1/oidc/status", oidcHTTP.status)
			mux.HandleFunc("GET /api/v1/oidc/login", oidcHTTP.login)
			mux.HandleFunc("GET /api/v1/oidc/callback", oidcHTTP.callback)
			mux.Handle("GET /api/v1/oidc/configuration", identityHTTP.requireSession(identityHTTP.requireLocalAdministrator(http.HandlerFunc(oidcHTTP.configurations))))
			mux.Handle("PUT /api/v1/oidc/configuration", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireLocalAdministrator(http.HandlerFunc(oidcHTTP.saveDraft)))))
			mux.Handle("GET /api/v1/oidc/configuration/test", identityHTTP.requireBrowserSession(identityHTTP.requireLocalAdministrator(http.HandlerFunc(oidcHTTP.test))))
			mux.Handle("POST /api/v1/oidc/configuration/activation", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireLocalAdministrator(http.HandlerFunc(oidcHTTP.activate)))))
		}
	}
	if options.Devices != nil && options.Identity != nil {
		handler := deviceHandler{devices: options.Devices, logger: logger}
		// Le scan et la récupération du résultat prouvent la possession du code
		// court. La création et la confirmation restent des gestes du navigateur.
		mux.HandleFunc("POST /api/v1/device-pairings/claim", handler.claimPairing)
		mux.HandleFunc("GET /api/v1/device-pairings/result", handler.pairingResult)
		mux.Handle("POST /api/v1/device-pairings", identityHTTP.requireSameOrigin(identityHTTP.requireBrowserSession(http.HandlerFunc(handler.createPairing))))
		mux.Handle("GET /api/v1/device-pairings/{pairingID}", identityHTTP.requireBrowserSession(http.HandlerFunc(handler.getPairing)))
		mux.Handle("POST /api/v1/device-pairings/{pairingID}/confirmation", identityHTTP.requireSameOrigin(identityHTTP.requireBrowserSession(http.HandlerFunc(handler.confirmPairing))))
		mux.Handle("DELETE /api/v1/device-pairings/{pairingID}", identityHTTP.requireSameOrigin(identityHTTP.requireBrowserSession(http.HandlerFunc(handler.cancelPairing))))
		mux.Handle("GET /api/v1/devices", identityHTTP.requireSession(http.HandlerFunc(handler.list)))
		mux.Handle("PATCH /api/v1/devices/{deviceID}", identityHTTP.requireSameOrigin(identityHTTP.requireSession(http.HandlerFunc(handler.update))))
		mux.Handle("DELETE /api/v1/devices/{deviceID}", identityHTTP.requireSameOrigin(identityHTTP.requireSession(http.HandlerFunc(handler.revoke))))
	}
	if options.ControlPlane != nil && options.Identity != nil {
		handler := controlPlaneHandler{controlPlane: options.ControlPlane, logger: logger}
		mux.Handle("GET /api/v1/targets", identityHTTP.requireSession(http.HandlerFunc(handler.listTargets)))
		mux.Handle("POST /api/v1/targets", identityHTTP.requireSameOrigin(identityHTTP.requireSession(http.HandlerFunc(handler.createTarget))))
		mux.Handle("PATCH /api/v1/targets/{targetID}", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.updateTarget)))))
		mux.Handle("DELETE /api/v1/targets/{targetID}", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.archiveTarget)))))
		mux.Handle("POST /api/v1/targets/{targetID}/restoration", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.restoreTarget)))))
		mux.Handle("POST /api/v1/targets/{targetID}/sources", identityHTTP.requireSameOrigin(identityHTTP.requireSession(http.HandlerFunc(handler.createSource))))
		mux.Handle("PATCH /api/v1/sources/{sourceID}", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.updateSource)))))
		mux.Handle("DELETE /api/v1/sources/{sourceID}", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.deleteSource)))))
		mux.Handle("GET /api/v1/targets/{targetID}/observations", identityHTTP.requireSession(http.HandlerFunc(handler.listObservations)))
		mux.HandleFunc("POST /api/v1/heartbeat/{token}", handler.receiveHeartbeat)
	}
	if options.Metrics != nil && options.Identity != nil {
		handler := metricsHandler{metrics: options.Metrics, logger: logger}
		mux.Handle("GET /api/v1/metrics/targets", identityHTTP.requireSession(http.HandlerFunc(handler.list)))
		mux.Handle("GET /api/v1/targets/{targetID}/metrics", identityHTTP.requireSession(http.HandlerFunc(handler.target)))
	}
	if options.Indicators != nil && options.Identity != nil {
		handler := indicatorHandler{indicators: options.Indicators, logger: logger}
		mux.Handle("GET /api/v1/connectors/{connectorID}/indicator-configuration", identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.configuration))))
		mux.Handle("POST /api/v1/connectors/{connectorID}/indicator-configuration/preview", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.preview)))))
		mux.Handle("PUT /api/v1/connectors/{connectorID}/indicator-configuration", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.apply)))))
		mux.Handle("GET /api/v1/indicators/targets", identityHTTP.requireSession(http.HandlerFunc(handler.overview)))
		mux.Handle("GET /api/v1/indicators/catalog", identityHTTP.requireSession(http.HandlerFunc(handler.catalog)))
		mux.Handle("GET /api/v1/targets/{targetID}/indicators", identityHTTP.requireSession(http.HandlerFunc(handler.target)))
		mux.Handle("GET /api/v1/incidents/{incidentID}/indicators", identityHTTP.requireSession(http.HandlerFunc(handler.incident)))
		mux.Handle("GET /api/v1/me/indicator-pins", identityHTTP.requireSession(http.HandlerFunc(handler.pins)))
		mux.Handle("PUT /api/v1/me/indicator-pins", identityHTTP.requireSameOrigin(identityHTTP.requireSession(http.HandlerFunc(handler.setPins))))
	}
	if options.Connectors != nil && options.Identity != nil {
		handler := connectorHandler{connectors: options.Connectors, logger: logger}
		mux.Handle("GET /api/v1/connectors", identityHTTP.requireSession(http.HandlerFunc(handler.list)))
		mux.Handle("POST /api/v1/connectors/zabbix/preview", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.previewZabbix)))))
		mux.Handle("POST /api/v1/connectors/zabbix/import", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.importZabbix)))))
		mux.Handle("POST /api/v1/connectors/uptime-kuma/preview", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.previewUptimeKuma)))))
		mux.Handle("POST /api/v1/connectors/uptime-kuma/import", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.importUptimeKuma)))))
		mux.Handle("POST /api/v1/connectors/patchmon/preview", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.previewPatchMon)))))
		mux.Handle("POST /api/v1/connectors/patchmon/import", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.importPatchMon)))))
		mux.Handle("POST /api/v1/connectors/argus/preview", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.previewArgus)))))
		mux.Handle("POST /api/v1/connectors/argus/import", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.importArgus)))))
		mux.Handle("POST /api/v1/connectors/{connectorID}/preview", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.previewExisting)))))
		mux.Handle("POST /api/v1/connectors/{connectorID}/suspension", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.suspend)))))
		mux.Handle("DELETE /api/v1/connectors/{connectorID}/suspension", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.resume)))))
		mux.Handle("DELETE /api/v1/connectors/{connectorID}", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.remove)))))
	}
	if options.Webhooks != nil {
		handler := webhookHandler{webhooks: options.Webhooks, logger: logger}
		mux.HandleFunc("POST /api/v1/webhooks/{publicID}", handler.receive)
		if options.Identity != nil {
			mux.Handle("POST /api/v1/connectors/generic-webhook", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.create)))))
			mux.Handle("GET /api/v1/connectors/{connectorID}/quarantine", identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.quarantine))))
			mux.Handle("POST /api/v1/connectors/{connectorID}/quarantine/{quarantineID}/approve", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.approve)))))
		}
	}
	if options.Incidents != nil && options.Identity != nil {
		handler := incidentHandler{incidents: options.Incidents, logger: logger}
		mux.Handle("GET /api/v1/incidents", identityHTTP.requireSession(http.HandlerFunc(handler.list)))
		mux.Handle("GET /api/v1/incidents/history", identityHTTP.requireSession(http.HandlerFunc(handler.history)))
		mux.Handle("GET /api/v1/incidents/{incidentID}", identityHTTP.requireSession(http.HandlerFunc(handler.get)))
		mux.Handle("POST /api/v1/incidents/{incidentID}/acknowledgement", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireAnyRole([]string{"administrator", "operator"}, http.HandlerFunc(handler.acknowledge)))))
		mux.Handle("POST /api/v1/incidents/{incidentID}/signals/{signalID}/invalidation", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireAnyRole([]string{"administrator", "operator"}, http.HandlerFunc(handler.invalidateSignal)))))
	}
	if options.Bursts != nil && options.Identity != nil {
		handler := burstHandler{bursts: options.Bursts, logger: logger}
		mux.Handle("GET /api/v1/incident-bursts", identityHTTP.requireSession(http.HandlerFunc(handler.list)))
		mux.Handle("GET /api/v1/incident-bursts/{burstID}", identityHTTP.requireSession(http.HandlerFunc(handler.get)))
		mux.Handle("POST /api/v1/incident-bursts/{burstID}/acknowledgement", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireAnyRole([]string{"administrator", "operator"}, http.HandlerFunc(handler.acknowledge)))))
	}
	if options.Maintenances != nil && options.Identity != nil {
		handler := maintenanceHandler{maintenances: options.Maintenances, logger: logger}
		mux.Handle("GET /api/v1/maintenances", identityHTTP.requireSession(http.HandlerFunc(handler.list)))
		mux.Handle("POST /api/v1/maintenances", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireAnyRole([]string{"administrator", "operator"}, http.HandlerFunc(handler.create)))))
		mux.Handle("POST /api/v1/maintenances/{maintenanceID}/cancellation", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireAnyRole([]string{"administrator", "operator"}, http.HandlerFunc(handler.cancel)))))
	}
	if options.Notifications != nil && options.Identity != nil {
		handler := notificationHandler{notifications: options.Notifications, logger: logger}
		mux.Handle("GET /api/v1/notification-channels", identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.list))))
		mux.Handle("POST /api/v1/notification-channels/mattermost", identityHTTP.requireSameOrigin(identityHTTP.requireSession(identityHTTP.requireRole("administrator", http.HandlerFunc(handler.createMattermost)))))
		// La boîte de réception n'est pas une administration : chacun lit la
		// sienne, quel que soit son rôle.
		mux.Handle("GET /api/v1/notifications", identityHTTP.requireSession(http.HandlerFunc(handler.inbox)))
		mux.Handle("DELETE /api/v1/notifications", identityHTTP.requireSameOrigin(identityHTTP.requireSession(http.HandlerFunc(handler.dismiss))))
		mux.Handle("POST /api/v1/notifications/read", identityHTTP.requireSameOrigin(identityHTTP.requireSession(http.HandlerFunc(handler.markRead))))
	}
	if options.Events != nil && options.Identity != nil {
		handler := realtimeHandler{events: options.Events, logger: logger}
		mux.Handle("GET /api/v1/events", identityHTTP.requireSameOrigin(identityHTTP.requireSession(http.HandlerFunc(handler.stream))))
	}
	if options.SystemHealth != nil && options.Identity != nil {
		handler := systemHealthHandler{health: options.SystemHealth, logger: logger}
		mux.Handle("GET /api/v1/system/health", identityHTTP.requireSession(http.HandlerFunc(handler.snapshot)))
	}
	if options.Reconciliations != nil && options.Identity != nil {
		handler := reconciliationHandler{service: options.Reconciliations, logger: logger}
		admin := func(next http.Handler) http.Handler {
			return identityHTTP.requireSession(identityHTTP.requireRole("administrator", next))
		}
		adminMutation := func(next http.Handler) http.Handler {
			return identityHTTP.requireSameOrigin(admin(next))
		}
		mux.Handle("GET /api/v1/target-reconciliation/suggestions", admin(http.HandlerFunc(handler.suggestions)))
		mux.Handle("POST /api/v1/target-reconciliation/preview", adminMutation(http.HandlerFunc(handler.previewTargets)))
		mux.Handle("POST /api/v1/source-reassignments/preview", adminMutation(http.HandlerFunc(handler.previewSource)))
		mux.Handle("GET /api/v1/target-reconciliation/operations", admin(http.HandlerFunc(handler.operations)))
		mux.Handle("POST /api/v1/target-reconciliation/operations", adminMutation(http.HandlerFunc(handler.enqueue)))
		mux.Handle("POST /api/v1/target-reconciliation/suggestions/{suggestionID}/rejection", adminMutation(http.HandlerFunc(handler.reject)))
		mux.Handle("POST /api/v1/target-reconciliation/suggestions/{suggestionID}/snooze", adminMutation(http.HandlerFunc(handler.snooze)))
		mux.Handle("GET /api/v1/targets/{targetID}/resolution", identityHTTP.requireSession(http.HandlerFunc(handler.resolveTarget)))
		mux.Handle("GET /api/v1/targets/{targetID}/reconciliation-activity", identityHTTP.requireSession(http.HandlerFunc(handler.targetActivity)))
	}

	if options.WebDir != "" {
		mux.Handle("/", newSPAHandler(options.WebDir))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		})
	}

	return &http.Server{
		Addr:              options.Address,
		Handler:           secureHeaders(requestLogger(logger, mux)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
		MaxHeaderBytes:    64 * 1024,
	}
}

func readinessHandler(pinger Pinger, service string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if pinger == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "unavailable", "service": service, "reason": "database not configured",
			})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pinger.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"status": "unavailable", "service": service, "reason": "database unreachable",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready", "service": service})
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil && !errors.Is(err, context.Canceled) {
		slog.Default().Error("encode HTTP response", "error", err)
	}
}

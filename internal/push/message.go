package push

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"time"
)

type Presentation struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type Message struct {
	Version          int          `json:"version"`
	EventKind        string       `json:"event_kind"`
	IncidentID       string       `json:"incident_id"`
	BurstID          string       `json:"burst_id,omitempty"`
	Revision         int          `json:"revision,omitempty"`
	PresentationMode string       `json:"presentation_mode,omitempty"`
	Severity         string       `json:"severity"`
	IncidentCount    int          `json:"incident_count,omitempty"`
	AffectedTargets  int          `json:"affected_target_count,omitempty"`
	MaxAffected      int          `json:"max_affected_targets,omitempty"`
	BurstStatus      string       `json:"burst_status,omitempty"`
	BurstExtended    bool         `json:"burst_extended,omitempty"`
	OccurredAt       time.Time    `json:"occurred_at"`
	InstanceURL      string       `json:"instance_url"`
	Presentation     Presentation `json:"presentation"`
}

func messageFor(delivery Delivery, publicURL string) Message {
	return Message{
		Version: 1, EventKind: delivery.EventKind, IncidentID: delivery.IncidentID,
		BurstID: delivery.BurstID, Revision: delivery.Revision,
		PresentationMode: delivery.PresentationMode,
		Severity:         delivery.Severity, IncidentCount: delivery.IncidentCount,
		AffectedTargets: delivery.AffectedTargets, MaxAffected: delivery.MaxAffected,
		BurstStatus: delivery.BurstStatus, BurstExtended: delivery.BurstExtended,
		OccurredAt:   delivery.OccurredAt,
		InstanceURL:  strings.TrimSuffix(publicURL, "/"),
		Presentation: presentationFor(delivery),
	}
}

func presentationFor(delivery Delivery) Presentation {
	english := delivery.Locale == "en"
	switch delivery.NotificationContent {
	case "masked":
		if english {
			return Presentation{Title: "CairnOps", Body: "Open CairnOps to view the update."}
		}
		return Presentation{Title: "CairnOps", Body: "Ouvrez CairnOps pour consulter la mise à jour."}
	case "discreet":
		if delivery.EventKind == "resolved" {
			if english {
				return Presentation{Title: "CairnOps", Body: "An incident has been resolved."}
			}
			return Presentation{Title: "CairnOps", Body: "Un Incident est résolu."}
		}
		if english {
			return Presentation{Title: "CairnOps", Body: "An incident requires attention."}
		}
		return Presentation{Title: "CairnOps", Body: "Un Incident demande votre attention."}
	default:
		if delivery.EventKind == "resolved" {
			if english {
				return Presentation{Title: delivery.TargetName, Body: delivery.NatureLabel + " resolved"}
			}
			return Presentation{Title: delivery.TargetName, Body: delivery.NatureLabel + " résolu"}
		}
		return Presentation{Title: delivery.TargetName, Body: delivery.NatureLabel}
	}
}

func collapseKey(incidentID string) string {
	digest := sha256.Sum256([]byte(incidentID))
	return base64.RawURLEncoding.EncodeToString(digest[:16])
}

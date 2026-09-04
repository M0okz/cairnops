package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maximumMattermostResponse = 64 * 1024

type MattermostClient struct{ client *http.Client }

func NewMattermostClient(client *http.Client) *MattermostClient {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &MattermostClient{client: client}
}

func (client *MattermostClient) Test(ctx context.Context, webhookURL string) error {
	return client.post(ctx, webhookURL, map[string]any{
		"username": "CairnOps",
		"text":     "✅ **CairnOps est connecté.** Les Incidents correspondant aux Gravités choisies seront publiés ici.",
	})
}

func (client *MattermostClient) Send(ctx context.Context, webhookURL string, message Message) error {
	resolved := message.EventKind == "resolved"
	color, label, icon := severityPresentation(string(message.Severity))
	title := fmt.Sprintf("%s [%s] %s — %s", icon, label, message.TargetName, message.NatureLabel)
	if resolved {
		color, title = "#39d98a", fmt.Sprintf("✅ [RÉSOLU] %s — %s", message.TargetName, message.NatureLabel)
	}
	fields := []map[string]any{
		{"short": true, "title": "Gravité", "value": label},
		{"short": true, "title": "État", "value": map[bool]string{true: "Résolu", false: "Actif"}[resolved]},
	}
	text := "La supervision serveur conserve les preuves liées à cet Incident."
	if resolved {
		text = "L’Incident est résolu. Cette notification est envoyée au même canal que son ouverture."
		if message.MaxAffected > 1 {
			text = fmt.Sprintf("L’Incident est résolu après avoir affecté jusqu’à %d Cibles. Ses Atteintes et leurs Preuves restent consultables.", message.MaxAffected)
		}
	}
	if message.PublicURL != "" {
		link := message.PublicURL + "/incidents?incident=" + url.QueryEscape(message.IncidentID)
		text += " [Ouvrir CairnOps](" + link + ")"
	}
	return client.post(ctx, webhookURL, map[string]any{
		"username": "CairnOps",
		"attachments": []map[string]any{{
			"color": color, "title": title, "text": text, "fields": fields,
			"footer": "Incident " + message.IncidentID,
		}},
	})
}

func severityPresentation(severity string) (color, label, icon string) {
	switch severity {
	case "critical":
		return "#ff5d5d", "CRITIQUE", "🚨"
	case "major":
		return "#ff8a3d", "MAJEURE", "⚠️"
	case "warning":
		return "#e9b949", "AVERTISSEMENT", "⚠️"
	default:
		return "#7aa2f7", "INFORMATION", "ℹ️"
	}
}

func (client *MattermostClient) post(ctx context.Context, webhookURL string, payload map[string]any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode Mattermost payload: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create Mattermost request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "CairnOps/1 Mattermost")
	response, err := client.client.Do(request)
	if err != nil {
		return fmt.Errorf("contact Mattermost: %w", err)
	}
	defer response.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, maximumMattermostResponse+1))
	if readErr != nil {
		return fmt.Errorf("read Mattermost response: %w", readErr)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail := strings.TrimSpace(string(responseBody))
		if len(detail) > 240 {
			detail = detail[:240]
		}
		return fmt.Errorf("Mattermost returned HTTP %d%s", response.StatusCode, map[bool]string{true: ": " + detail, false: ""}[detail != ""])
	}
	return nil
}

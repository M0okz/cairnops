package uptimekuma

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
)

type BootstrapSession struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
	Version  string `json:"version"`
}

type ManagedCredential struct {
	APIKey string `json:"api_key"`
	ID     string `json:"id"`
}

type socketSession struct {
	connection    *websocket.Conn
	nextAckID     int
	pendingEvents []socketEvent
}

func (client *Client) PrepareBootstrap(ctx context.Context, address, username, password, secondFactor string) (Inspection, BootstrapSession, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	endpoint, err := NormalizeEndpoint(address)
	if err != nil {
		return Inspection{}, BootstrapSession{}, err
	}
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 4096 || password == "" || len(password) > 4096 {
		return Inspection{}, BootstrapSession{}, fmt.Errorf("username and password must each contain between 1 and 4096 characters")
	}
	if len(strings.TrimSpace(secondFactor)) > 32 {
		return Inspection{}, BootstrapSession{}, fmt.Errorf("two-factor code must contain at most 32 characters")
	}
	socket, err := client.openSocket(ctx, endpoint)
	if err != nil {
		return Inspection{}, BootstrapSession{}, err
	}
	defer socket.connection.CloseNow()
	var login struct {
		OK            bool   `json:"ok"`
		Message       string `json:"msg"`
		Token         string `json:"token"`
		TokenRequired bool   `json:"tokenRequired"`
	}
	if err := socket.emitAck(ctx, "login", map[string]string{
		"username": username, "password": password, "token": strings.TrimSpace(secondFactor),
	}, &login); err != nil {
		return Inspection{}, BootstrapSession{}, fmt.Errorf("authenticate Uptime Kuma installer: %w", err)
	}
	if login.TokenRequired {
		return Inspection{}, BootstrapSession{}, fmt.Errorf("authenticate Uptime Kuma installer: a two-factor code is required")
	}
	if !login.OK || strings.TrimSpace(login.Token) == "" {
		return Inspection{}, BootstrapSession{}, fmt.Errorf("authenticate Uptime Kuma installer: credentials were rejected")
	}
	inspection, version, err := socket.readInventory(ctx, endpoint)
	if err != nil {
		return Inspection{}, BootstrapSession{}, err
	}
	if !supportedBootstrapVersion(version) {
		return Inspection{}, BootstrapSession{}, fmt.Errorf("Uptime Kuma %s is not supported for automatic API-key setup", version)
	}
	return inspection, BootstrapSession{Endpoint: endpoint, Token: login.Token, Version: version}, nil
}

func (client *Client) Provision(ctx context.Context, session BootstrapSession) (ManagedCredential, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if !supportedBootstrapVersion(session.Version) || strings.TrimSpace(session.Token) == "" {
		return ManagedCredential{}, fmt.Errorf("Uptime Kuma bootstrap session is incomplete or unsupported")
	}
	socket, err := client.openSocket(ctx, session.Endpoint)
	if err != nil {
		return ManagedCredential{}, err
	}
	defer socket.connection.CloseNow()
	var login struct {
		OK bool `json:"ok"`
	}
	if err := socket.emitAck(ctx, "loginByToken", session.Token, &login); err != nil || !login.OK {
		if err == nil {
			err = fmt.Errorf("session token was rejected")
		}
		return ManagedCredential{}, fmt.Errorf("resume Uptime Kuma installer session: %w", err)
	}
	var created struct {
		OK      bool   `json:"ok"`
		Message string `json:"msg"`
		Key     string `json:"key"`
		KeyID   any    `json:"keyID"`
	}
	key := map[string]any{
		"name": "CairnOps", "active": 1,
		"expires": time.Now().UTC().AddDate(10, 0, 0).Format("2006-01-02 15:04:05"),
	}
	if err := socket.emitAck(ctx, "addAPIKey", key, &created); err != nil {
		return ManagedCredential{}, fmt.Errorf("create Uptime Kuma API key: %w", err)
	}
	id := strings.TrimSpace(fmt.Sprint(created.KeyID))
	if !created.OK || strings.TrimSpace(created.Key) == "" || id == "" || id == "<nil>" {
		return ManagedCredential{}, fmt.Errorf("create Uptime Kuma API key: invalid response")
	}
	return ManagedCredential{APIKey: created.Key, ID: id}, nil
}

func (client *Client) Revoke(ctx context.Context, session BootstrapSession, credentialID string) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	socket, err := client.openSocket(ctx, session.Endpoint)
	if err != nil {
		return err
	}
	defer socket.connection.CloseNow()
	var login struct {
		OK bool `json:"ok"`
	}
	if err := socket.emitAck(ctx, "loginByToken", session.Token, &login); err != nil || !login.OK {
		if err == nil {
			err = fmt.Errorf("session token was rejected")
		}
		return fmt.Errorf("resume Uptime Kuma installer session: %w", err)
	}
	var deleted struct {
		OK bool `json:"ok"`
	}
	keyID, err := strconv.Atoi(strings.TrimSpace(credentialID))
	if err != nil {
		return fmt.Errorf("Uptime Kuma API key identity is invalid")
	}
	if err := socket.emitAck(ctx, "deleteAPIKey", keyID, &deleted); err != nil {
		return fmt.Errorf("delete Uptime Kuma API key: %w", err)
	}
	if !deleted.OK {
		return fmt.Errorf("delete Uptime Kuma API key: remote deletion failed")
	}
	return nil
}

func (client *Client) openSocket(ctx context.Context, endpoint string) (*socketSession, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse Uptime Kuma endpoint: %w", err)
	}
	parsed.Scheme = map[string]string{"http": "ws", "https": "wss"}[parsed.Scheme]
	parsed.Path = strings.TrimSuffix(parsed.Path, "/metrics") + "/socket.io/"
	parsed.RawQuery = "EIO=4&transport=websocket"
	connection, response, err := websocket.Dial(ctx, parsed.String(), &websocket.DialOptions{
		HTTPClient: client.http,
		HTTPHeader: http.Header{"User-Agent": []string{"CairnOps connector setup"}},
	})
	if err != nil {
		if response != nil {
			return nil, fmt.Errorf("open Uptime Kuma setup channel: HTTP %d", response.StatusCode)
		}
		return nil, fmt.Errorf("open Uptime Kuma setup channel: %w", err)
	}
	session := &socketSession{connection: connection, nextAckID: 1}
	if err := session.waitForEngineOpen(ctx); err != nil {
		connection.CloseNow()
		return nil, err
	}
	if err := connection.Write(ctx, websocket.MessageText, []byte("40")); err != nil {
		connection.CloseNow()
		return nil, fmt.Errorf("join Uptime Kuma setup channel: %w", err)
	}
	// Waiting for the first info event avoids emitting login before Uptime
	// Kuma has registered its authenticated Socket.IO handlers.
	if _, err := session.waitForEvent(ctx, "info"); err != nil {
		connection.CloseNow()
		return nil, fmt.Errorf("initialize Uptime Kuma setup channel: %w", err)
	}
	return session, nil
}

func (session *socketSession) waitForEngineOpen(ctx context.Context) error {
	for {
		message, err := session.readPacket(ctx)
		if err != nil {
			return fmt.Errorf("read Uptime Kuma setup handshake: %w", err)
		}
		if strings.HasPrefix(message, "0") {
			return nil
		}
	}
}

func (session *socketSession) emitAck(ctx context.Context, event string, argument any, target any) error {
	id := session.nextAckID
	session.nextAckID++
	payload, err := json.Marshal([]any{event, argument})
	if err != nil {
		return err
	}
	packet := "42" + strconv.Itoa(id) + string(payload)
	if err := session.connection.Write(ctx, websocket.MessageText, []byte(packet)); err != nil {
		return err
	}
	prefix := "43" + strconv.Itoa(id)
	for {
		message, err := session.readPacket(ctx)
		if err != nil {
			return err
		}
		if !strings.HasPrefix(message, prefix) {
			if event, ok := parseSocketEvent(message); ok {
				session.pendingEvents = append(session.pendingEvents, event)
			}
			continue
		}
		var values []json.RawMessage
		if err := json.Unmarshal([]byte(strings.TrimPrefix(message, prefix)), &values); err != nil || len(values) != 1 {
			return fmt.Errorf("invalid Uptime Kuma acknowledgement")
		}
		if target != nil {
			return json.Unmarshal(values[0], target)
		}
		return nil
	}
}

func (session *socketSession) readInventory(ctx context.Context, endpoint string) (Inspection, string, error) {
	type remoteMonitor struct {
		ID       any    `json:"id"`
		Name     string `json:"name"`
		Type     string `json:"type"`
		URL      string `json:"url"`
		Hostname string `json:"hostname"`
		Port     any    `json:"port"`
	}
	monitors := map[string]remoteMonitor{}
	statuses := map[string]struct {
		Status int
		Ping   *int
	}{}
	heartbeatsSeen := map[string]bool{}
	version := ""
	monitorListSeen := false
	for {
		event, err := session.nextEvent(ctx)
		if err != nil {
			return Inspection{}, "", fmt.Errorf("read Uptime Kuma inventory: %w", err)
		}
		switch event.Name {
		case "info":
			if len(event.Arguments) > 0 {
				var info struct {
					Version string `json:"version"`
				}
				_ = json.Unmarshal(event.Arguments[0], &info)
				if info.Version != "" {
					version = info.Version
				}
			}
		case "monitorList":
			if len(event.Arguments) > 0 {
				if json.Unmarshal(event.Arguments[0], &monitors) == nil {
					monitorListSeen = true
				}
			}
		case "heartbeatList":
			if len(event.Arguments) < 2 {
				continue
			}
			var monitorID any
			var beats []struct {
				Status int      `json:"status"`
				Ping   *float64 `json:"ping"`
			}
			if json.Unmarshal(event.Arguments[0], &monitorID) != nil || json.Unmarshal(event.Arguments[1], &beats) != nil {
				continue
			}
			id := fmt.Sprint(monitorID)
			heartbeatsSeen[id] = true
			if len(beats) > 0 {
				latest := beats[len(beats)-1]
				var ping *int
				if latest.Ping != nil && *latest.Ping >= 0 {
					value := int(*latest.Ping + 0.5)
					ping = &value
				}
				statuses[id] = struct {
					Status int
					Ping   *int
				}{latest.Status, ping}
			}
		}
		if version != "" && monitorListSeen && len(heartbeatsSeen) >= len(monitors) {
			break
		}
	}
	result := make([]Monitor, 0, len(monitors))
	for key, remote := range monitors {
		id := strings.TrimSpace(fmt.Sprint(remote.ID))
		if id == "" || id == "<nil>" {
			id = key
		}
		status := 2
		var ping *int
		if observed, ok := statuses[id]; ok {
			status, ping = observed.Status, observed.Ping
		}
		port := strings.TrimSpace(fmt.Sprint(remote.Port))
		if port == "<nil>" {
			port = ""
		}
		result = append(result, Monitor{
			ID: id, Name: strings.TrimSpace(remote.Name), Type: strings.TrimSpace(remote.Type),
			URL: remote.URL, Hostname: remote.Hostname, Port: port,
			Status: status, ResponseMilliseconds: ping,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if strings.EqualFold(result[i].Name, result[j].Name) {
			return result[i].ID < result[j].ID
		}
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return Inspection{
		Endpoint: endpoint, EncryptedTransport: strings.HasPrefix(endpoint, "https://"), Monitors: result,
	}, version, nil
}

type socketEvent struct {
	Name      string
	Arguments []json.RawMessage
}

func (session *socketSession) waitForEvent(ctx context.Context, name string) (socketEvent, error) {
	for {
		event, err := session.nextEvent(ctx)
		if err != nil {
			return socketEvent{}, err
		}
		if event.Name == name {
			return event, nil
		}
	}
}

func (session *socketSession) nextEvent(ctx context.Context) (socketEvent, error) {
	if len(session.pendingEvents) > 0 {
		event := session.pendingEvents[0]
		session.pendingEvents = session.pendingEvents[1:]
		return event, nil
	}
	for {
		message, err := session.readPacket(ctx)
		if err != nil {
			return socketEvent{}, err
		}
		if event, ok := parseSocketEvent(message); ok {
			return event, nil
		}
	}
}

func parseSocketEvent(message string) (socketEvent, bool) {
	if !strings.HasPrefix(message, "42") {
		return socketEvent{}, false
	}
	start := strings.Index(message, "[")
	if start < 0 {
		return socketEvent{}, false
	}
	var values []json.RawMessage
	if err := json.Unmarshal([]byte(message[start:]), &values); err != nil || len(values) == 0 {
		return socketEvent{}, false
	}
	var name string
	if json.Unmarshal(values[0], &name) != nil {
		return socketEvent{}, false
	}
	return socketEvent{Name: name, Arguments: values[1:]}, true
}

func (session *socketSession) readPacket(ctx context.Context) (string, error) {
	for {
		_, payload, err := session.connection.Read(ctx)
		if err != nil {
			return "", err
		}
		message := string(payload)
		if message == "2" {
			if err := session.connection.Write(ctx, websocket.MessageText, []byte("3")); err != nil {
				return "", err
			}
			continue
		}
		return message, nil
	}
}

func supportedBootstrapVersion(version string) bool {
	parts := strings.SplitN(strings.TrimPrefix(strings.TrimSpace(version), "v"), ".", 2)
	major, err := strconv.Atoi(parts[0])
	return err == nil && major == 2
}

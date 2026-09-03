package patchmon

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type BootstrapSession struct {
	Endpoint string `json:"endpoint"`
	Token    string `json:"token"`
}

type ManagedCredential struct {
	Credentials Credentials `json:"credentials"`
	ID          string      `json:"id"`
}

func (client *Client) PrepareBootstrap(ctx context.Context, address, username, password, secondFactor string) (Inspection, BootstrapSession, error) {
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
	base, err := managementBase(endpoint)
	if err != nil {
		return Inspection{}, BootstrapSession{}, err
	}
	var login struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
		RequiresTFA bool   `json:"requiresTfa"`
		TFATicket   string `json:"tfaTicket"`
	}
	if err := client.doManagementJSON(ctx, http.MethodPost, base+"/auth/login", "", map[string]string{
		"username": username, "password": password,
	}, &login, http.StatusOK); err != nil {
		return Inspection{}, BootstrapSession{}, fmt.Errorf("authenticate PatchMon installer: %w", err)
	}
	if login.RequiresTFA {
		secondFactor = strings.TrimSpace(secondFactor)
		if secondFactor == "" || login.TFATicket == "" {
			return Inspection{}, BootstrapSession{}, fmt.Errorf("authenticate PatchMon installer: a two-factor code is required")
		}
		if err := client.doManagementJSON(ctx, http.MethodPost, base+"/auth/verify-tfa", "", map[string]any{
			"username": username, "token": secondFactor, "remember_me": false, "tfa_ticket": login.TFATicket,
		}, &login, http.StatusOK); err != nil {
			return Inspection{}, BootstrapSession{}, fmt.Errorf("verify PatchMon second factor: %w", err)
		}
	}
	if login.Token == "" {
		login.Token = login.AccessToken
	}
	if strings.TrimSpace(login.Token) == "" {
		return Inspection{}, BootstrapSession{}, fmt.Errorf("authenticate PatchMon installer: invalid session response")
	}
	session := BootstrapSession{Endpoint: endpoint, Token: login.Token}
	temporary, err := client.Provision(ctx, session)
	if err != nil {
		return Inspection{}, BootstrapSession{}, err
	}
	inspection, inspectErr := client.Inspect(ctx, endpoint, temporary.Credentials)
	revokeErr := client.Revoke(ctx, session, temporary.ID)
	if inspectErr != nil {
		return Inspection{}, BootstrapSession{}, inspectErr
	}
	if revokeErr != nil {
		return Inspection{}, BootstrapSession{}, fmt.Errorf("remove PatchMon preview token: %w", revokeErr)
	}
	return inspection, session, nil
}

func (client *Client) Provision(ctx context.Context, session BootstrapSession) (ManagedCredential, error) {
	base, err := managementBase(session.Endpoint)
	if err != nil {
		return ManagedCredential{}, err
	}
	if strings.TrimSpace(session.Token) == "" {
		return ManagedCredential{}, fmt.Errorf("PatchMon bootstrap session is incomplete")
	}
	var nonce [6]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return ManagedCredential{}, fmt.Errorf("name PatchMon API token: %w", err)
	}
	var response struct {
		Token struct {
			ID     string `json:"id"`
			Key    string `json:"token_key"`
			Secret string `json:"token_secret"`
		} `json:"token"`
	}
	payload := map[string]any{
		"token_name": "CairnOps " + fmt.Sprintf("%x", nonce[:]),
		"metadata":   map[string]string{"integration_type": "api", "managed_by": "cairnops"},
		"scopes":     map[string][]string{"host": {"get"}},
	}
	if err := client.doManagementJSON(ctx, http.MethodPost, base+"/auto-enrollment/tokens", session.Token, payload, &response, http.StatusCreated); err != nil {
		return ManagedCredential{}, fmt.Errorf("create PatchMon API token: %w", err)
	}
	if strings.TrimSpace(response.Token.ID) == "" || strings.TrimSpace(response.Token.Key) == "" || strings.TrimSpace(response.Token.Secret) == "" {
		return ManagedCredential{}, fmt.Errorf("create PatchMon API token: invalid response")
	}
	return ManagedCredential{
		ID:          response.Token.ID,
		Credentials: Credentials{Key: response.Token.Key, Secret: response.Token.Secret},
	}, nil
}

func (client *Client) Revoke(ctx context.Context, session BootstrapSession, credentialID string) error {
	base, err := managementBase(session.Endpoint)
	if err != nil {
		return err
	}
	credentialID = strings.TrimSpace(credentialID)
	if credentialID == "" {
		return fmt.Errorf("PatchMon API token identity is missing")
	}
	if err := client.doManagementJSON(
		ctx, http.MethodDelete, base+"/auto-enrollment/tokens/"+url.PathEscape(credentialID),
		session.Token, nil, nil, http.StatusOK,
	); err != nil {
		return fmt.Errorf("delete PatchMon API token: %w", err)
	}
	return nil
}

func managementBase(endpoint string) (string, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse PatchMon endpoint: %w", err)
	}
	const suffix = "/api/v1/api/hosts"
	if !strings.HasSuffix(parsed.Path, suffix) {
		return "", fmt.Errorf("PatchMon endpoint has an unsupported management path")
	}
	parsed.Path = strings.TrimSuffix(parsed.Path, suffix) + "/api/v1"
	parsed.RawPath, parsed.RawQuery, parsed.Fragment = "", "", ""
	return strings.TrimSuffix(parsed.String(), "/"), nil
}

func (client *Client) doManagementJSON(ctx context.Context, method, endpoint, bearer string, payload, target any, acceptedStatus int) error {
	if client == nil || client.http == nil {
		return fmt.Errorf("HTTP client is not configured")
	}
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != acceptedStatus {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("remote server returned HTTP %d", response.StatusCode)
	}
	if target == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maximumResponseBytes))
		return nil
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

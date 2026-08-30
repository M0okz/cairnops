package oidcauth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	oidclib "github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
)

type claimedFlow struct {
	purpose        string
	nonce          string
	verifierSealed string
	returnTo       string
	configuration  Configuration
}

func (service *Service) Begin(ctx context.Context, purpose, returnTo string) (Authorization, error) {
	if purpose != "login" && purpose != "test" {
		return Authorization{}, fmt.Errorf("%w: unknown flow purpose", ErrInvalidInput)
	}
	state := "active"
	if purpose == "test" {
		state = "draft"
	}
	configuration, err := service.configurationByState(ctx, state)
	if err != nil {
		return Authorization{}, err
	}
	provider, err := service.provider(ctx, configuration)
	if err != nil {
		return Authorization{}, fmt.Errorf("discover OIDC provider: %w", err)
	}

	flowState, err := randomValue(32)
	if err != nil {
		return Authorization{}, err
	}
	nonce, err := randomValue(32)
	if err != nil {
		return Authorization{}, err
	}
	verifier := oauth2.GenerateVerifier()
	sealedVerifier, err := service.secrets.Seal([]byte(verifier), flowVerifierPurpose)
	if err != nil {
		return Authorization{}, fmt.Errorf("seal OIDC verifier: %w", err)
	}
	returnTo = safeReturnTo(returnTo)
	digest := sha256.Sum256([]byte(flowState))
	if _, err := service.pool.Exec(ctx, `
		INSERT INTO cairnops_oidc_flows (
			purpose, configuration_id, state_digest, nonce,
			code_verifier_sealed, return_to, expires_at
		) VALUES ($1, $2::uuid, $3, $4, $5, $6, $7)
	`, purpose, configuration.ID, digest[:], nonce, sealedVerifier, returnTo, service.now().UTC().Add(flowLifetime)); err != nil {
		return Authorization{}, fmt.Errorf("create OIDC flow: %w", err)
	}

	config, err := service.oauthConfiguration(ctx, configuration, provider)
	if err != nil {
		return Authorization{}, err
	}
	return Authorization{URL: config.AuthCodeURL(
		flowState,
		oauth2.AccessTypeOffline,
		oauth2.S256ChallengeOption(verifier),
		oidclib.Nonce(nonce),
		oauth2.SetAuthURLParam("prompt", "consent"),
	), State: flowState}, nil
}

func (service *Service) Complete(ctx context.Context, state, code string) (Completion, error) {
	if state == "" || len(state) > 512 || code == "" || len(code) > 8192 {
		return Completion{}, ErrInvalidFlow
	}
	flow, err := service.claimFlow(ctx, state)
	if err != nil {
		return Completion{}, err
	}
	verifier, err := service.secrets.Open(flow.verifierSealed, flowVerifierPurpose)
	if err != nil {
		return Completion{}, fmt.Errorf("open OIDC verifier: %w", err)
	}
	provider, err := service.provider(ctx, flow.configuration)
	if err != nil {
		return Completion{}, fmt.Errorf("discover OIDC provider: %w", err)
	}
	oauthConfig, err := service.oauthConfiguration(ctx, flow.configuration, provider)
	if err != nil {
		return Completion{}, err
	}
	token, err := oauthConfig.Exchange(service.clientContext(ctx), code, oauth2.VerifierOption(string(verifier)))
	if err != nil {
		return Completion{}, fmt.Errorf("exchange OIDC code: %w", err)
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return Completion{}, fmt.Errorf("%w: the provider did not return an ID token", ErrInvalidFlow)
	}
	idToken, err := provider.Verifier(&oidclib.Config{ClientID: flow.configuration.ClientID}).Verify(ctx, rawIDToken)
	if err != nil {
		return Completion{}, fmt.Errorf("verify OIDC ID token: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(flow.nonce)) != 1 {
		return Completion{}, fmt.Errorf("%w: OIDC nonce mismatch", ErrInvalidFlow)
	}
	if idToken.AccessTokenHash != "" {
		if err := idToken.VerifyAccessToken(token.AccessToken); err != nil {
			return Completion{}, fmt.Errorf("verify OIDC access token: %w", err)
		}
	}
	userInfo, err := provider.UserInfo(service.clientContext(ctx), oauth2.StaticTokenSource(token))
	if err != nil {
		return Completion{}, fmt.Errorf("read OIDC UserInfo: %w", err)
	}
	if userInfo.Subject == "" || userInfo.Subject != idToken.Subject {
		return Completion{}, fmt.Errorf("%w: UserInfo subject does not match the ID token", ErrInvalidFlow)
	}
	claims := make(map[string]json.RawMessage)
	if err := userInfo.Claims(&claims); err != nil {
		return Completion{}, fmt.Errorf("decode OIDC UserInfo claims: %w", err)
	}
	_, role, roleErr := groupsAndRole(claims, flow.configuration)
	if roleErr != nil {
		if flow.purpose == "login" {
			_ = service.suspendByIdentity(ctx, flow.configuration.Issuer, idToken.Subject, "groups_not_authorized")
		}
		return Completion{}, roleErr
	}
	if token.RefreshToken == "" && flow.purpose == "test" {
		return Completion{}, fmt.Errorf("%w: le fournisseur n’a pas délivré de refresh token", ErrNotAuthorized)
	}

	if flow.purpose == "test" {
		result, err := service.pool.Exec(ctx, `
			UPDATE cairnops_oidc_configurations
			SET tested_at = now(), tested_subject = $2, updated_at = now()
			WHERE id = $1::uuid AND state = 'draft'
		`, flow.configuration.ID, idToken.Subject)
		if err != nil {
			return Completion{}, fmt.Errorf("record OIDC test: %w", err)
		}
		if result.RowsAffected() != 1 {
			return Completion{}, ErrInvalidFlow
		}
		return Completion{Purpose: "test", ReturnTo: flow.returnTo}, nil
	}

	name := displayName(claims, userInfo.Subject)
	userID, err := service.applyExternalLogin(ctx, flow.configuration, idToken.Subject, name, role, token.RefreshToken)
	if err != nil {
		return Completion{}, err
	}
	session, err := service.sessions.NewSession(ctx, userID)
	if err != nil {
		return Completion{}, fmt.Errorf("open OIDC session: %w", err)
	}
	return Completion{Purpose: "login", ReturnTo: flow.returnTo, Session: session}, nil
}

func (service *Service) claimFlow(ctx context.Context, rawState string) (claimedFlow, error) {
	digest := sha256.Sum256([]byte(rawState))
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return claimedFlow{}, fmt.Errorf("begin OIDC callback: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var flow claimedFlow
	var configurationID string
	err = tx.QueryRow(ctx, `
		UPDATE cairnops_oidc_flows
		SET consumed_at = now()
		WHERE state_digest = $1 AND consumed_at IS NULL AND expires_at > now()
		RETURNING purpose, configuration_id::text, nonce, code_verifier_sealed, return_to
	`, digest[:]).Scan(&flow.purpose, &configurationID, &flow.nonce, &flow.verifierSealed, &flow.returnTo)
	if errors.Is(err, pgx.ErrNoRows) {
		return claimedFlow{}, ErrInvalidFlow
	}
	if err != nil {
		return claimedFlow{}, fmt.Errorf("consume OIDC flow: %w", err)
	}
	flow.configuration, err = scanConfiguration(tx.QueryRow(ctx, `
		SELECT id::text, state, label, issuer, client_id, client_secret_sealed,
		       groups_claim, administrator_groups, operator_groups, observer_groups,
		       tested_at, activated_at, created_at, updated_at
		FROM cairnops_oidc_configurations WHERE id = $1::uuid
	`, configurationID))
	if err != nil {
		return claimedFlow{}, fmt.Errorf("read OIDC flow configuration: %w", err)
	}
	if (flow.purpose == "login" && flow.configuration.State != "active") || (flow.purpose == "test" && flow.configuration.State != "draft") {
		return claimedFlow{}, ErrInvalidFlow
	}
	if err := tx.Commit(ctx); err != nil {
		return claimedFlow{}, fmt.Errorf("commit OIDC callback claim: %w", err)
	}
	return flow, nil
}

func (service *Service) configurationByState(ctx context.Context, state string) (Configuration, error) {
	configuration, err := scanConfiguration(service.pool.QueryRow(ctx, `
		SELECT id::text, state, label, issuer, client_id, client_secret_sealed,
		       groups_claim, administrator_groups, operator_groups, observer_groups,
		       tested_at, activated_at, created_at, updated_at
		FROM cairnops_oidc_configurations WHERE state = $1
	`, state))
	if errors.Is(err, pgx.ErrNoRows) {
		return Configuration{}, ErrNotConfigured
	}
	if err != nil {
		return Configuration{}, fmt.Errorf("read %s OIDC configuration: %w", state, err)
	}
	return configuration, nil
}

func (service *Service) provider(ctx context.Context, configuration Configuration) (*oidclib.Provider, error) {
	return oidclib.NewProvider(service.clientContext(ctx), configuration.Issuer)
}

func (service *Service) clientContext(ctx context.Context) context.Context {
	return oidclib.ClientContext(ctx, service.httpClient)
}

func (service *Service) oauthConfiguration(ctx context.Context, configuration Configuration, provider *oidclib.Provider) (oauth2.Config, error) {
	secret, err := service.secrets.Open(configuration.clientSecretSealed, clientSecretPurpose)
	if err != nil {
		return oauth2.Config{}, fmt.Errorf("open OIDC client secret: %w", err)
	}
	publicURL := service.publicURL
	if publicURL == "" {
		publicURL = "http://localhost:8080"
	}
	return oauth2.Config{
		ClientID: configuration.ClientID, ClientSecret: string(secret),
		Endpoint: provider.Endpoint(), RedirectURL: publicURL + "/api/v1/oidc/callback",
		Scopes: []string{oidclib.ScopeOpenID, "profile", oidclib.ScopeOfflineAccess},
	}, nil
}

func (service *Service) applyExternalLogin(ctx context.Context, configuration Configuration, subject, name, role, refreshToken string) (string, error) {
	sealedRefresh := ""
	var err error
	if refreshToken != "" {
		sealedRefresh, err = service.secrets.Seal([]byte(refreshToken), refreshTokenPurpose)
		if err != nil {
			return "", fmt.Errorf("seal OIDC refresh token: %w", err)
		}
	}
	tx, err := service.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin OIDC identity update: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	identityDigest := sha256.Sum256([]byte(configuration.Issuer + "\x00" + subject))
	identityLock := int64(binary.BigEndian.Uint64(identityDigest[:8]))
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, identityLock); err != nil {
		return "", fmt.Errorf("lock OIDC identity: %w", err)
	}

	var userID, priorRole string
	var deactivatedAt *time.Time
	err = tx.QueryRow(ctx, `
		SELECT users.id::text, users.role, users.deactivated_at
		FROM cairnops_oidc_identities identities
		JOIN cairnops_users users ON users.id = identities.user_id
		WHERE identities.issuer = $1 AND identities.subject = $2
		FOR UPDATE OF identities, users
	`, configuration.Issuer, subject).Scan(&userID, &priorRole, &deactivatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		if sealedRefresh == "" {
			return "", fmt.Errorf("%w: le fournisseur n’a pas délivré de refresh token", ErrNotAuthorized)
		}
		err = tx.QueryRow(ctx, `
			INSERT INTO cairnops_users (
				username, display_name, password_hash, role, authorization_regime
			) VALUES ($1, $2, NULL, $3, 'external')
			RETURNING id::text
		`, externalUsername(configuration.Issuer, subject), name, role).Scan(&userID)
		if err != nil {
			return "", fmt.Errorf("create external User: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO cairnops_oidc_identities (
				user_id, issuer, subject, refresh_token_sealed,
				last_verified_at, sync_due_at
			) VALUES ($1::uuid, $2, $3, $4, now(), $5)
		`, userID, configuration.Issuer, subject, sealedRefresh, nextSync(service.now().UTC(), subject)); err != nil {
			return "", fmt.Errorf("create OIDC identity: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("find OIDC identity: %w", err)
	} else {
		if deactivatedAt != nil {
			return "", fmt.Errorf("%w: cet Utilisateur a été désactivé", ErrNotAuthorized)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_users
			SET display_name = $2, role = $3, external_suspended_at = NULL,
			    external_suspension_reason = '', updated_at = now()
			WHERE id = $1::uuid
		`, userID, name, role); err != nil {
			return "", fmt.Errorf("update external User: %w", err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_oidc_identities
			SET refresh_token_sealed = CASE WHEN $2 = '' THEN refresh_token_sealed ELSE $2 END,
			    last_verified_at = now(),
			    sync_due_at = $3, sync_lease_owner = NULL, sync_lease_until = NULL,
			    last_sync_error = '', updated_at = now()
			WHERE user_id = $1::uuid
		`, userID, sealedRefresh, nextSync(service.now().UTC(), subject)); err != nil {
			return "", fmt.Errorf("update OIDC identity: %w", err)
		}
		if priorRole != role {
			if _, err := tx.Exec(ctx, `UPDATE cairnops_sessions SET revoked_at = now() WHERE user_id = $1::uuid AND revoked_at IS NULL`, userID); err != nil {
				return "", fmt.Errorf("revoke sessions after OIDC role change: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit OIDC identity: %w", err)
	}
	return userID, nil
}

func (service *Service) suspendByIdentity(ctx context.Context, issuer, subject, reason string) error {
	_, err := service.pool.Exec(ctx, `
		WITH suspended AS (
			UPDATE cairnops_users users
			SET external_suspended_at = coalesce(external_suspended_at, now()),
			    external_suspension_reason = $3, updated_at = now()
			FROM cairnops_oidc_identities identities
			WHERE identities.user_id = users.id AND identities.issuer = $1
			  AND identities.subject = $2 AND users.authorization_regime = 'external'
			RETURNING users.id
		)
		UPDATE cairnops_sessions sessions SET revoked_at = now()
		FROM suspended WHERE sessions.user_id = suspended.id AND sessions.revoked_at IS NULL
	`, issuer, subject, reason)
	if err != nil {
		return fmt.Errorf("suspend external User: %w", err)
	}
	return nil
}

func safeReturnTo(raw string) string {
	if raw == "" {
		return "/"
	}
	parsed, err := url.Parse(raw)
	if err != nil || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") || strings.Contains(raw, "\\") || strings.HasPrefix(parsed.Path, "//") || strings.Contains(parsed.Path, "\\") || parsed.IsAbs() || parsed.Host != "" {
		return "/"
	}
	return raw
}

func nextSync(now time.Time, subject string) time.Time {
	digest := sha256.Sum256([]byte(subject))
	jitter := time.Duration(int(digest[0])%61-30) * time.Second
	return now.Add(5*time.Minute + jitter)
}

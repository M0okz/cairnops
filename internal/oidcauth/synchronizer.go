package oidcauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/oauth2"
)

const (
	synchronizationPoll  = 15 * time.Second
	synchronizationLease = 90 * time.Second
	providerGrace        = 12 * time.Hour
)

type dueIdentity struct {
	userID             string
	subject            string
	refreshTokenSealed string
	lastVerifiedAt     time.Time
	configuration      Configuration
}

type Synchronizer struct {
	service *Service
	owner   string
	logger  *slog.Logger
}

func NewSynchronizer(service *Service, owner string, logger *slog.Logger) *Synchronizer {
	if logger == nil {
		logger = slog.Default()
	}
	return &Synchronizer{service: service, owner: owner, logger: logger}
}

func (syncer *Synchronizer) Run(ctx context.Context) error {
	ticker := time.NewTicker(synchronizationPoll)
	defer ticker.Stop()
	for {
		if err := syncer.drain(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (syncer *Synchronizer) drain(ctx context.Context) error {
	if _, err := syncer.service.pool.Exec(ctx, `DELETE FROM cairnops_oidc_flows WHERE expires_at < now() - interval '1 hour'`); err != nil {
		return fmt.Errorf("clean expired OIDC flows: %w", err)
	}
	for count := 0; count < 20; count++ {
		identity, found, err := syncer.claim(ctx)
		if err != nil {
			return err
		}
		if !found {
			return nil
		}
		if err := syncer.synchronize(ctx, identity); err != nil {
			syncer.logger.Warn("OIDC identity synchronization failed", "user_id", identity.userID, "error", err)
		}
	}
	return nil
}

func (syncer *Synchronizer) claim(ctx context.Context) (dueIdentity, bool, error) {
	tx, err := syncer.service.pool.Begin(ctx)
	if err != nil {
		return dueIdentity{}, false, fmt.Errorf("begin OIDC synchronization claim: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var identity dueIdentity
	err = tx.QueryRow(ctx, `
		SELECT identities.user_id::text, identities.subject,
		       identities.refresh_token_sealed, identities.last_verified_at,
		       configuration.id::text, configuration.state, configuration.label,
		       configuration.issuer, configuration.client_id,
		       configuration.client_secret_sealed, configuration.groups_claim,
		       configuration.administrator_groups, configuration.operator_groups,
		       configuration.observer_groups, configuration.tested_at,
		       configuration.activated_at, configuration.created_at,
		       configuration.updated_at
		FROM cairnops_oidc_identities identities
		JOIN cairnops_oidc_configurations configuration
		  ON configuration.state = 'active' AND configuration.issuer = identities.issuer
		WHERE identities.sync_due_at <= now()
		  AND (identities.sync_lease_until IS NULL OR identities.sync_lease_until < now())
		ORDER BY identities.sync_due_at
		FOR UPDATE OF identities SKIP LOCKED
		LIMIT 1
	`).Scan(
		&identity.userID, &identity.subject, &identity.refreshTokenSealed, &identity.lastVerifiedAt,
		&identity.configuration.ID, &identity.configuration.State, &identity.configuration.Label,
		&identity.configuration.Issuer, &identity.configuration.ClientID,
		&identity.configuration.clientSecretSealed, &identity.configuration.GroupsClaim,
		&identity.configuration.Groups.Administrator, &identity.configuration.Groups.Operator,
		&identity.configuration.Groups.Observer, &identity.configuration.TestedAt,
		&identity.configuration.ActivatedAt, &identity.configuration.CreatedAt,
		&identity.configuration.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return dueIdentity{}, false, nil
	}
	if err != nil {
		return dueIdentity{}, false, fmt.Errorf("claim due OIDC identity: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_oidc_identities
		SET sync_lease_owner = $2, sync_lease_until = $3
		WHERE user_id = $1::uuid
	`, identity.userID, syncer.owner, syncer.service.now().UTC().Add(synchronizationLease)); err != nil {
		return dueIdentity{}, false, fmt.Errorf("lease OIDC identity: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return dueIdentity{}, false, fmt.Errorf("commit OIDC synchronization claim: %w", err)
	}
	identity.configuration.ClientSecretConfigured = true
	return identity, true, nil
}

func (syncer *Synchronizer) synchronize(ctx context.Context, identity dueIdentity) error {
	refreshToken, err := syncer.service.secrets.Open(identity.refreshTokenSealed, refreshTokenPurpose)
	if err != nil {
		return syncer.fail(ctx, identity, fmt.Errorf("open OIDC refresh token: %w", err), true, "refresh_token_invalid")
	}
	provider, err := syncer.service.provider(ctx, identity.configuration)
	if err != nil {
		return syncer.fail(ctx, identity, fmt.Errorf("discover OIDC provider: %w", err), false, "provider_unavailable")
	}
	config, err := syncer.service.oauthConfiguration(ctx, identity.configuration, provider)
	if err != nil {
		return syncer.fail(ctx, identity, err, true, "configuration_invalid")
	}
	stale := &oauth2.Token{RefreshToken: string(refreshToken), Expiry: time.Unix(1, 0)}
	token, err := config.TokenSource(syncer.service.clientContext(ctx), stale).Token()
	if err != nil {
		return syncer.fail(ctx, identity, fmt.Errorf("refresh OIDC token: %w", err), explicitProviderRefusal(err), "refresh_refused")
	}
	userInfo, err := provider.UserInfo(syncer.service.clientContext(ctx), oauth2.StaticTokenSource(token))
	if err != nil {
		return syncer.fail(ctx, identity, fmt.Errorf("read OIDC UserInfo: %w", err), explicitProviderRefusal(err), "userinfo_refused")
	}
	if userInfo.Subject == "" || userInfo.Subject != identity.subject {
		return syncer.fail(ctx, identity, fmt.Errorf("OIDC UserInfo subject changed"), true, "subject_changed")
	}
	claims := make(map[string]json.RawMessage)
	if err := userInfo.Claims(&claims); err != nil {
		return syncer.fail(ctx, identity, fmt.Errorf("decode OIDC UserInfo: %w", err), true, "claims_invalid")
	}
	_, role, err := groupsAndRole(claims, identity.configuration)
	if err != nil {
		return syncer.fail(ctx, identity, err, true, "groups_not_authorized")
	}
	rotated := token.RefreshToken
	if rotated == "" {
		rotated = string(refreshToken)
	}
	sealedRefresh, err := syncer.service.secrets.Seal([]byte(rotated), refreshTokenPurpose)
	if err != nil {
		return syncer.fail(ctx, identity, fmt.Errorf("seal rotated OIDC refresh token: %w", err), true, "refresh_token_invalid")
	}
	return syncer.succeed(ctx, identity, displayName(claims, identity.subject), role, sealedRefresh)
}

func (syncer *Synchronizer) succeed(ctx context.Context, identity dueIdentity, name, role, sealedRefresh string) error {
	tx, err := syncer.service.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin OIDC synchronization result: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var priorRole string
	if err := tx.QueryRow(ctx, `SELECT role FROM cairnops_users WHERE id = $1::uuid FOR UPDATE`, identity.userID).Scan(&priorRole); err != nil {
		return fmt.Errorf("lock synchronized User: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE cairnops_users
		SET display_name = $2, role = $3, external_suspended_at = NULL,
		    external_suspension_reason = '', updated_at = now()
		WHERE id = $1::uuid AND authorization_regime = 'external'
	`, identity.userID, name, role); err != nil {
		return fmt.Errorf("update synchronized User: %w", err)
	}
	result, err := tx.Exec(ctx, `
		UPDATE cairnops_oidc_identities
		SET refresh_token_sealed = $3, last_verified_at = now(), sync_due_at = $4,
		    sync_lease_owner = NULL, sync_lease_until = NULL,
		    last_sync_error = '', updated_at = now()
		WHERE user_id = $1::uuid AND sync_lease_owner = $2
	`, identity.userID, syncer.owner, sealedRefresh, nextSync(syncer.service.now().UTC(), identity.subject))
	if err != nil {
		return fmt.Errorf("record OIDC synchronization success: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("OIDC synchronization lease was lost")
	}
	if priorRole != role {
		if _, err := tx.Exec(ctx, `UPDATE cairnops_sessions SET revoked_at = now() WHERE user_id = $1::uuid AND revoked_at IS NULL`, identity.userID); err != nil {
			return fmt.Errorf("revoke sessions after synchronized role change: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit OIDC synchronization result: %w", err)
	}
	return nil
}

func (syncer *Synchronizer) fail(ctx context.Context, identity dueIdentity, cause error, immediate bool, reason string) error {
	now := syncer.service.now().UTC()
	suspend := immediate || !now.Before(identity.lastVerifiedAt.Add(providerGrace))
	tx, err := syncer.service.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin OIDC synchronization failure: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, `
		UPDATE cairnops_oidc_identities
		SET last_sync_error = $3, sync_due_at = $4,
		    sync_lease_owner = NULL, sync_lease_until = NULL, updated_at = now()
		WHERE user_id = $1::uuid AND sync_lease_owner = $2
	`, identity.userID, syncer.owner, truncateError(cause), nextSync(now, identity.subject))
	if err != nil {
		return fmt.Errorf("record OIDC synchronization failure: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("OIDC synchronization lease was lost")
	}
	if suspend {
		if _, err := tx.Exec(ctx, `
			UPDATE cairnops_users
			SET external_suspended_at = coalesce(external_suspended_at, now()),
			    external_suspension_reason = $2, updated_at = now()
			WHERE id = $1::uuid AND authorization_regime = 'external'
		`, identity.userID, reason); err != nil {
			return fmt.Errorf("suspend external User after synchronization: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE cairnops_sessions SET revoked_at = now() WHERE user_id = $1::uuid AND revoked_at IS NULL`, identity.userID); err != nil {
			return fmt.Errorf("revoke suspended OIDC sessions: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit OIDC synchronization failure: %w", err)
	}
	return cause
}

func explicitProviderRefusal(err error) bool {
	var retrieve *oauth2.RetrieveError
	if errors.As(err, &retrieve) {
		if retrieve.Response == nil {
			return false
		}
		switch retrieve.Response.StatusCode {
		case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden:
			return true
		default:
			return false
		}
	}
	message := strings.ToLower(err.Error())
	return strings.HasPrefix(message, "400 ") || strings.HasPrefix(message, "401 ") || strings.HasPrefix(message, "403 ")
}

func truncateError(err error) string {
	message := err.Error()
	if len(message) > 1000 {
		return message[:1000]
	}
	return message
}

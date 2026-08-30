-- Un Utilisateur relève d'une seule autorité. Les comptes historiques restent
-- locaux ; un compte externe n'a jamais de mot de passe CairnOps.
ALTER TABLE cairnops_users
    ADD COLUMN authorization_regime text NOT NULL DEFAULT 'local'
        CHECK (authorization_regime IN ('local', 'external')),
    ADD COLUMN external_suspended_at timestamptz,
    ADD COLUMN external_suspension_reason text NOT NULL DEFAULT '';

ALTER TABLE cairnops_users ALTER COLUMN password_hash DROP NOT NULL;

ALTER TABLE cairnops_users
    ADD CONSTRAINT cairnops_users_authority_check CHECK (
        (authorization_regime = 'local' AND password_hash IS NOT NULL
            AND external_suspended_at IS NULL AND external_suspension_reason = '')
        OR
        (authorization_regime = 'external' AND password_hash IS NULL)
    );

ALTER TABLE cairnops_users
    ADD CONSTRAINT cairnops_users_id_regime_key UNIQUE (id, authorization_regime);

DROP INDEX cairnops_users_active_administrators_idx;
CREATE INDEX cairnops_users_active_local_administrators_idx
    ON cairnops_users (role)
    WHERE deactivated_at IS NULL
      AND authorization_regime = 'local'
      AND role = 'administrator';

-- Chaque enregistrement est une version complète de la configuration. Un
-- brouillon raté ne touche donc jamais à la version active.
CREATE TABLE cairnops_oidc_configurations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    state text NOT NULL CHECK (state IN ('draft', 'active', 'retired')),
    label text NOT NULL CHECK (length(btrim(label)) BETWEEN 1 AND 80),
    issuer text NOT NULL,
    client_id text NOT NULL CHECK (length(btrim(client_id)) BETWEEN 1 AND 255),
    client_secret_sealed text NOT NULL,
    groups_claim text NOT NULL DEFAULT 'groups'
        CHECK (groups_claim ~ '^[A-Za-z_][A-Za-z0-9_.-]{0,127}$'),
    administrator_groups text[] NOT NULL DEFAULT '{}',
    operator_groups text[] NOT NULL DEFAULT '{}',
    observer_groups text[] NOT NULL DEFAULT '{}',
    tested_at timestamptz,
    tested_subject text NOT NULL DEFAULT '',
    activated_at timestamptz,
    created_by uuid NOT NULL REFERENCES cairnops_users(id) ON DELETE RESTRICT,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (cardinality(administrator_groups) + cardinality(operator_groups) + cardinality(observer_groups) > 0),
    CHECK ((state = 'active') = (activated_at IS NOT NULL))
);

CREATE UNIQUE INDEX cairnops_oidc_one_draft_idx
    ON cairnops_oidc_configurations (state) WHERE state = 'draft';
CREATE UNIQUE INDEX cairnops_oidc_one_active_idx
    ON cairnops_oidc_configurations (state) WHERE state = 'active';

-- L'identité durable ne dépend d'aucun attribut humain et mutable. Elle porte
-- le secret nécessaire à la réévaluation périodique des groupes.
CREATE TABLE cairnops_oidc_identities (
    user_id uuid PRIMARY KEY,
    authorization_regime text NOT NULL DEFAULT 'external' CHECK (authorization_regime = 'external'),
    issuer text NOT NULL,
    subject text NOT NULL CHECK (length(subject) BETWEEN 1 AND 512),
    refresh_token_sealed text NOT NULL,
    last_verified_at timestamptz NOT NULL,
    sync_due_at timestamptz NOT NULL,
    sync_lease_owner text,
    sync_lease_until timestamptz,
    last_sync_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (user_id, authorization_regime)
        REFERENCES cairnops_users(id, authorization_regime) ON DELETE RESTRICT,
    UNIQUE (issuer, subject)
);

CREATE INDEX cairnops_oidc_identities_due_idx
    ON cairnops_oidc_identities (sync_due_at)
    WHERE sync_lease_until IS NULL;

-- Les paramètres anti-rejeu restent côté serveur et expirent vite. Le
-- navigateur ne transporte que l'état aléatoire et le retour du fournisseur.
CREATE TABLE cairnops_oidc_flows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    purpose text NOT NULL CHECK (purpose IN ('login', 'test')),
    configuration_id uuid NOT NULL REFERENCES cairnops_oidc_configurations(id) ON DELETE CASCADE,
    state_digest bytea NOT NULL UNIQUE CHECK (octet_length(state_digest) = 32),
    nonce text NOT NULL,
    code_verifier_sealed text NOT NULL,
    return_to text NOT NULL DEFAULT '/',
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX cairnops_oidc_flows_expiry_idx ON cairnops_oidc_flows (expires_at);

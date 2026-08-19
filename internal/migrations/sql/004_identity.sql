CREATE TABLE cairnops_installation (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    initialized_at timestamptz
);

INSERT INTO cairnops_installation (singleton) VALUES (true);

CREATE TABLE cairnops_users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username text NOT NULL CHECK (username ~ '^[a-z0-9][a-z0-9._-]{2,63}$'),
    display_name text NOT NULL CHECK (length(btrim(display_name)) BETWEEN 1 AND 100),
    password_hash text NOT NULL,
    role text NOT NULL CHECK (role IN ('administrator', 'operator', 'observer')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX cairnops_users_username_idx ON cairnops_users (lower(username));

CREATE TABLE cairnops_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES cairnops_users(id) ON DELETE CASCADE,
    token_digest bytea NOT NULL CHECK (octet_length(token_digest) = 32),
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX cairnops_sessions_token_digest_idx ON cairnops_sessions (token_digest);
CREATE INDEX cairnops_sessions_user_active_idx
    ON cairnops_sessions (user_id, expires_at DESC)
    WHERE revoked_at IS NULL;

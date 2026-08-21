-- Chaque compagnon mobile possède sa propre identité. Le secret
-- d'authentification n'est conservé que sous forme d'empreinte ; le destinataire
-- opaque du Relais Push reste scellé avec la clé maîtresse de l'instance.
CREATE TABLE cairnops_devices (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES cairnops_users(id) ON DELETE CASCADE,
    name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 100),
    platform text NOT NULL CHECK (platform IN ('ios', 'android')),
    app_version text NOT NULL DEFAULT '' CHECK (length(app_version) <= 64),
    locale text NOT NULL DEFAULT 'fr' CHECK (locale IN ('fr', 'en')),
    notification_content text NOT NULL DEFAULT 'complete'
        CHECK (notification_content IN ('complete', 'discreet', 'masked')),
    encryption_public_key bytea NOT NULL CHECK (octet_length(encryption_public_key) = 32),
    push_recipient_sealed text NOT NULL CHECK (length(push_recipient_sealed) BETWEEN 32 AND 4096),
    token_digest bytea NOT NULL CHECK (octet_length(token_digest) = 32),
    last_seen_at timestamptz,
    push_disabled_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX cairnops_devices_token_digest_idx ON cairnops_devices (token_digest);
CREATE INDEX cairnops_devices_user_active_idx
    ON cairnops_devices (user_id, created_at DESC)
    WHERE revoked_at IS NULL;

-- Le code du QR n'est jamais persisté. Son empreinte autorise une seule
-- revendication, puis la confirmation Web produit un jeton d'appareil scellé
-- que le mobile ne peut retirer qu'une fois.
CREATE TABLE cairnops_device_pairings (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES cairnops_users(id) ON DELETE CASCADE,
    token_digest bytea NOT NULL CHECK (octet_length(token_digest) = 32),
    expires_at timestamptz NOT NULL,
    claimed_name text CHECK (claimed_name IS NULL OR length(btrim(claimed_name)) BETWEEN 1 AND 100),
    claimed_platform text CHECK (claimed_platform IS NULL OR claimed_platform IN ('ios', 'android')),
    claimed_app_version text CHECK (claimed_app_version IS NULL OR length(claimed_app_version) <= 64),
    claimed_locale text CHECK (claimed_locale IS NULL OR claimed_locale IN ('fr', 'en')),
    claimed_notification_content text CHECK (
        claimed_notification_content IS NULL
        OR claimed_notification_content IN ('complete', 'discreet', 'masked')
    ),
    claimed_encryption_public_key bytea CHECK (
        claimed_encryption_public_key IS NULL
        OR octet_length(claimed_encryption_public_key) = 32
    ),
    claimed_push_recipient_sealed text CHECK (
        claimed_push_recipient_sealed IS NULL
        OR length(claimed_push_recipient_sealed) BETWEEN 32 AND 4096
    ),
    claimed_at timestamptz,
    confirmed_at timestamptz,
    cancelled_at timestamptz,
    credential_consumed_at timestamptz,
    device_id uuid UNIQUE REFERENCES cairnops_devices(id) ON DELETE SET NULL,
    device_token_sealed text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (
        claimed_at IS NULL
        OR (
            claimed_name IS NOT NULL
            AND claimed_platform IS NOT NULL
            AND claimed_app_version IS NOT NULL
            AND claimed_locale IS NOT NULL
            AND claimed_notification_content IS NOT NULL
            AND claimed_encryption_public_key IS NOT NULL
            AND claimed_push_recipient_sealed IS NOT NULL
        )
    ),
    CHECK (confirmed_at IS NULL OR (claimed_at IS NOT NULL AND device_id IS NOT NULL)),
    CHECK (credential_consumed_at IS NULL OR confirmed_at IS NOT NULL)
);

CREATE UNIQUE INDEX cairnops_device_pairings_token_digest_idx
    ON cairnops_device_pairings (token_digest);
CREATE INDEX cairnops_device_pairings_user_recent_idx
    ON cairnops_device_pairings (user_id, created_at DESC);
CREATE INDEX cairnops_device_pairings_expiry_idx
    ON cairnops_device_pairings (expires_at)
    WHERE confirmed_at IS NULL AND cancelled_at IS NULL;

-- Une ligne par appareil et notification intégrée permet de reprendre un échec
-- sans renvoyer la même notification aux appareils déjà servis.
CREATE TABLE cairnops_push_outbox (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    device_id uuid NOT NULL REFERENCES cairnops_devices(id) ON DELETE CASCADE,
    inbox_id bigint NOT NULL REFERENCES cairnops_notification_inbox(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'delivered', 'failed', 'cancelled')),
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    lease_owner text,
    lease_until timestamptz,
    delivered_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (device_id, inbox_id)
);

CREATE INDEX cairnops_push_outbox_due_idx
    ON cairnops_push_outbox (next_attempt_at, id)
    WHERE status IN ('pending', 'failed');

-- Le worker sonde le Relais même quand aucune notification n'est due. La vue
-- Santé distingue ainsi un relais réellement joignable d'un canal simplement
-- silencieux.
CREATE TABLE cairnops_push_relay_status (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    configured boolean NOT NULL DEFAULT false,
    status text NOT NULL DEFAULT 'unavailable'
        CHECK (status IN ('operational', 'unavailable')),
    last_checked_at timestamptz,
    last_success_at timestamptz,
    last_error text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO cairnops_push_relay_status (singleton) VALUES (true);

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_kind_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_kind_check CHECK (kind IN (
    'target.changed',
    'source.changed',
    'observation.created',
    'component.heartbeat',
    'connector.changed',
    'incident.changed',
    'maintenance.changed',
    'notification.changed',
    'device.changed'
));

ALTER TABLE cairnops_events DROP CONSTRAINT cairnops_events_entity_type_check;
ALTER TABLE cairnops_events ADD CONSTRAINT cairnops_events_entity_type_check CHECK (
    entity_type IN (
        'target', 'source', 'component', 'connector', 'incident',
        'maintenance', 'notification', 'device'
    )
);

CREATE FUNCTION cairnops_device_changed_event()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    PERFORM cairnops_append_event(
        'device.changed',
        'device',
        CASE WHEN TG_OP = 'DELETE' THEN OLD.id::text ELSE NEW.id::text END
    );
    RETURN NULL;
END;
$$;

CREATE TRIGGER cairnops_devices_changed
AFTER INSERT OR UPDATE OF name, app_version, locale, notification_content,
    push_recipient_sealed, push_disabled_at, last_seen_at, revoked_at OR DELETE
ON cairnops_devices
FOR EACH ROW EXECUTE FUNCTION cairnops_device_changed_event();

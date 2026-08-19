-- Les notifications intégrées sont un Canal comme les autres.
--
-- Les faire passer par la boîte d'envoi leur donne, sans rien réécrire, ce que
-- Mattermost avait déjà : le routage par Gravité, l'annulation de ce qui n'est
-- pas encore parti dès l'Acquittement, la neutralisation par une fenêtre de
-- maintenance, et la Résolution adressée aux mêmes destinataires. Ce qui
-- change tient à la livraison : elle ne sort pas de l'instance, elle dépose
-- une entrée par personne.
ALTER TABLE cairnops_notification_channels DROP CONSTRAINT cairnops_notification_channels_kind_check;
ALTER TABLE cairnops_notification_channels
    ADD CONSTRAINT cairnops_notification_channels_kind_check
    CHECK (kind IN ('mattermost', 'in_app'));

-- Un Canal intégré n'a ni adresse ni secret : il ne sort pas de l'instance.
ALTER TABLE cairnops_notification_channels DROP CONSTRAINT cairnops_notification_channels_endpoint_check;
ALTER TABLE cairnops_notification_channels
    ADD CONSTRAINT cairnops_notification_channels_endpoint_check
    CHECK (
        (kind = 'in_app' AND endpoint = '')
        OR length(btrim(endpoint)) BETWEEN 1 AND 512
    );

ALTER TABLE cairnops_notification_channels DROP CONSTRAINT cairnops_notification_channels_credential_sealed_check;
ALTER TABLE cairnops_notification_channels
    ADD CONSTRAINT cairnops_notification_channels_credential_sealed_check
    CHECK (
        (kind = 'in_app' AND credential_sealed = '')
        OR length(credential_sealed) > 0
    );

-- Il n'y en a qu'un : « intégré » désigne l'instance elle-même, pas une
-- destination que l'on choisirait plusieurs fois.
CREATE UNIQUE INDEX cairnops_notification_channels_in_app_idx
    ON cairnops_notification_channels ((true))
    WHERE kind = 'in_app';

-- Une instance notifie dès son premier Incident : le Canal existe d'emblée,
-- avec les mêmes Gravités que celles proposées pour Mattermost. Un
-- Administrateur peut le suspendre comme n'importe quel autre.
INSERT INTO cairnops_notification_channels (
    kind, name, endpoint, credential_sealed, severities,
    enabled, status, encrypted_transport, last_checked_at, last_error
) VALUES (
    'in_app', 'Notifications intégrées', '', '',
    ARRAY['warning', 'major', 'critical']::text[],
    true, 'connected', true, now(), ''
);

-- Ce que chacun a reçu. Une entrée par personne et par événement : c'est elle
-- qui porte la lecture, et c'est en la relisant que la Résolution retrouve
-- exactement les destinataires de l'ouverture.
CREATE TABLE cairnops_notification_inbox (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES cairnops_users(id) ON DELETE CASCADE,
    incident_id uuid NOT NULL REFERENCES cairnops_incidents(id) ON DELETE CASCADE,
    target_id uuid REFERENCES cairnops_targets(id) ON DELETE SET NULL,
    event_kind text NOT NULL CHECK (event_kind IN ('firing', 'resolved')),
    target_name text NOT NULL,
    nature_label text NOT NULL,
    severity text NOT NULL CHECK (severity IN ('information', 'warning', 'major', 'critical')),
    occurred_at timestamptz NOT NULL,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, incident_id, event_kind)
);

-- Ce que l'écran demande le plus souvent : mes entrées, les non lues d'abord.
CREATE INDEX cairnops_notification_inbox_unread_idx
    ON cairnops_notification_inbox (user_id, occurred_at DESC)
    WHERE read_at IS NULL;

CREATE INDEX cairnops_notification_inbox_recent_idx
    ON cairnops_notification_inbox (user_id, occurred_at DESC);

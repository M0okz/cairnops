-- Les analyses de notes ne participent jamais aux incidents ni aux notifications.
CREATE TABLE cairnops_software_services (
    binding_id uuid PRIMARY KEY,
    installed_version text NOT NULL DEFAULT '',
    target_version text NOT NULL DEFAULT '',
    observed_at timestamptz,
    known boolean NOT NULL DEFAULT false,
    source jsonb NOT NULL DEFAULT '{}',
    confirmed_by uuid REFERENCES cairnops_users(id) ON DELETE SET NULL,
    confirmed_at timestamptz,
    revision bigint NOT NULL DEFAULT 1,
    next_check_at timestamptz NOT NULL DEFAULT now(),
    lease_token text,
    lease_until timestamptz,
    state text NOT NULL DEFAULT 'awaiting_source',
    last_error text NOT NULL DEFAULT '',
    checked_at timestamptz,
    collection jsonb,
    collection_revision bigint,
    content_hash text NOT NULL DEFAULT ''
);
CREATE TABLE cairnops_software_history (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    binding_id uuid NOT NULL REFERENCES cairnops_software_services(binding_id),
    installed_version text NOT NULL,
    target_version text NOT NULL,
    observed_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON cairnops_software_history(binding_id, id DESC);
CREATE TABLE cairnops_software_analyses (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    binding_id uuid NOT NULL REFERENCES cairnops_software_services(binding_id),
    revision bigint NOT NULL,
    installed_version text NOT NULL,
    target_version text NOT NULL,
    source jsonb NOT NULL,
    content_hash text NOT NULL,
    result jsonb NOT NULL,
    notes jsonb NOT NULL DEFAULT '[]',
    model text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON cairnops_software_analyses(binding_id, id DESC);
CREATE TABLE cairnops_software_ai (
    singleton boolean PRIMARY KEY DEFAULT true CHECK(singleton),
    enabled boolean NOT NULL DEFAULT false,
    endpoint text NOT NULL DEFAULT '',
    model text NOT NULL DEFAULT '',
    credential_sealed text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO cairnops_software_ai(singleton) VALUES (true);

CREATE FUNCTION cairnops_capture_software_version() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE valid boolean; changed boolean; previous cairnops_software_services%ROWTYPE;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM cairnops_connectors WHERE id=NEW.connector_id AND kind='argus') THEN RETURN NEW; END IF;
    INSERT INTO cairnops_software_services(binding_id) VALUES (NEW.id) ON CONFLICT DO NOTHING;
    SELECT * INTO previous FROM cairnops_software_services WHERE binding_id=NEW.id FOR UPDATE;
    valid := coalesce(NEW.metadata->>'unknown','false') <> 'true'
        AND coalesce(NEW.metadata->>'deployed_version','') <> ''
        AND coalesce(NEW.metadata->>'latest_version','') <> '';
    IF NOT valid THEN
        UPDATE cairnops_software_services SET known=false WHERE binding_id=NEW.id;
        RETURN NEW;
    END IF;
    changed := previous.installed_version IS DISTINCT FROM NEW.metadata->>'deployed_version'
        OR previous.target_version IS DISTINCT FROM NEW.metadata->>'latest_version';
    IF changed THEN
        INSERT INTO cairnops_software_history(binding_id,installed_version,target_version)
        VALUES(NEW.id,NEW.metadata->>'deployed_version',NEW.metadata->>'latest_version');
    END IF;
    UPDATE cairnops_software_services SET
        installed_version=NEW.metadata->>'deployed_version', target_version=NEW.metadata->>'latest_version',
        known=true, observed_at=now(), revision=revision+CASE WHEN changed THEN 1 ELSE 0 END,
        next_check_at=CASE WHEN changed THEN now() ELSE next_check_at END,
        state=CASE WHEN changed THEN CASE WHEN confirmed_at IS NULL THEN 'awaiting_source' ELSE 'pending' END ELSE state END
    WHERE binding_id=NEW.id;
    RETURN NEW;
END; $$;
CREATE TRIGGER cairnops_capture_software_version AFTER INSERT OR UPDATE OF metadata ON cairnops_connector_bindings
FOR EACH ROW EXECUTE FUNCTION cairnops_capture_software_version();
-- Le premier constat est daté de l'activation du suivi, pas d'un déploiement supposé.
UPDATE cairnops_connector_bindings b SET metadata=b.metadata FROM cairnops_connectors c WHERE b.connector_id=c.id AND c.kind='argus';

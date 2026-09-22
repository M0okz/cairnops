ALTER TABLE cairnops_software_services ADD COLUMN source_origin text NOT NULL DEFAULT 'manual'
 CHECK (source_origin IN ('manual','argus'));
-- Retry previously rejected model output with server-resolved evidence.
UPDATE cairnops_software_services SET next_check_at=now()
 WHERE state='retry' AND last_error LIKE 'invalid_ai_%';

CREATE OR REPLACE FUNCTION cairnops_capture_software_version() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE valid boolean; changed boolean; source_changed boolean; previous cairnops_software_services%ROWTYPE;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM cairnops_connectors WHERE id=NEW.connector_id AND kind='argus') THEN RETURN NEW; END IF;
    INSERT INTO cairnops_software_services(binding_id) VALUES (NEW.id) ON CONFLICT DO NOTHING;
    SELECT * INTO previous FROM cairnops_software_services WHERE binding_id=NEW.id FOR UPDATE;
    source_changed := TG_OP='UPDATE' AND previous.source_origin='argus'
        AND coalesce(nullif(OLD.metadata->>'release_source_url',''),OLD.metadata->>'version_url','')
         IS DISTINCT FROM coalesce(nullif(NEW.metadata->>'release_source_url',''),NEW.metadata->>'version_url','');
    IF source_changed THEN
        UPDATE cairnops_software_services SET revision=revision+1,state='pending',next_check_at=now() WHERE binding_id=NEW.id;
    END IF;
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

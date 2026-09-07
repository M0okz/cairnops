-- Presentation enrichment is independent of Nature identities and revisions.
-- Historical source text is never used to manufacture a structured meaning.
ALTER TABLE cairnops_incident_evidence
    ADD COLUMN alert_facts jsonb NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(alert_facts) = 'object');
ALTER TABLE cairnops_incidents ADD COLUMN alert_kind text NOT NULL DEFAULT '';
ALTER TABLE cairnops_notification_outbox ADD COLUMN alert_kind text NOT NULL DEFAULT '';
ALTER TABLE cairnops_notification_inbox ADD COLUMN alert_kind text NOT NULL DEFAULT '';

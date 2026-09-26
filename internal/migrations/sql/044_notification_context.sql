-- Contexte figé d'une notification : Intégrations, fait structuré unanime,
-- premières Ressources, hausse de Gravité et durée. La boîte intégrée et le
-- Push relisent ainsi ce qui a été livré. Les entrées existantes restent sans
-- contexte : rien n'est reconstitué après coup.
ALTER TABLE cairnops_notification_inbox
    ADD COLUMN context jsonb NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(context) = 'object');

-- Une Source apportée par une Intégration existe désormais comme Source de
-- signal à part entière : c'est elle qui porte les Observations dont se
-- déduisent Disponibilité, Couverture et latence. Elle reste la propriété de
-- son Intégration, jamais de CairnOps.
ALTER TABLE cairnops_signal_sources
    ADD COLUMN origin text NOT NULL DEFAULT 'native'
        CHECK (origin IN ('native', 'integration')),
    ADD COLUMN connector_binding_id uuid
        REFERENCES cairnops_connector_bindings(id) ON DELETE CASCADE;

ALTER TABLE cairnops_signal_sources DROP CONSTRAINT cairnops_signal_sources_kind_check;
ALTER TABLE cairnops_signal_sources ADD CONSTRAINT cairnops_signal_sources_kind_check CHECK (
    kind IN ('http', 'tcp', 'dns', 'icmp', 'heartbeat', 'zabbix', 'uptime_kuma', 'generic_webhook')
);

-- Un Contrôle natif n'a pas de liaison ; une Source d'Intégration en a
-- exactement une, et une liaison ne porte qu'une Source.
ALTER TABLE cairnops_signal_sources ADD CONSTRAINT cairnops_signal_sources_origin_binding_check CHECK (
    (origin = 'native' AND connector_binding_id IS NULL)
    OR (origin = 'integration' AND connector_binding_id IS NOT NULL)
);

CREATE UNIQUE INDEX cairnops_signal_sources_binding_idx
    ON cairnops_signal_sources (connector_binding_id)
    WHERE connector_binding_id IS NOT NULL;

-- Le worker n'exécute que les Contrôles natifs : l'index des Sources dues les
-- isole, et le scheduler ne réclame plus rien d'autre.
DROP INDEX cairnops_signal_sources_due_idx;
CREATE INDEX cairnops_signal_sources_due_idx
    ON cairnops_signal_sources (next_run_at)
    WHERE enabled AND origin = 'native';

-- Les Cibles déjà importées reçoivent leur Source rétroactivement, à la
-- cadence de synchronisation de leur Connecteur. Leur mesure commence donc au
-- premier cycle qui suit la migration, sans réécrire un passé non observé.
INSERT INTO cairnops_signal_sources (
    target_id, name, kind, origin, connector_binding_id, enabled,
    interval_seconds, timeout_milliseconds, config, created_at
)
SELECT binding.target_id,
       left(coalesce(nullif(btrim(binding.external_name), ''), 'Source importée'), 160),
       connector.kind, 'integration', binding.id, connector.status <> 'disabled',
       connector.sync_interval_seconds, 1000, '{}'::jsonb, now()
FROM cairnops_connector_bindings binding
JOIN cairnops_connectors connector ON connector.id = binding.connector_id;

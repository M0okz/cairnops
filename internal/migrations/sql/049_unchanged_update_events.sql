-- Un UPDATE qui ne change rien de visible n'annonce plus rien.
--
-- Les cycles de synchronisation réécrivent leurs lignes toutes les 30 secondes,
-- le plus souvent à l'identique : la Preuve ne gagne qu'un last_seen_at, le
-- Connecteur repasse « connected » sans erreur, le détecteur de doublons
-- reconduit la même suggestion. Chaque réécriture ajoutait pourtant une ligne
-- à cairnops_events, et chaque client rechargeait alors des listes complètes.
-- Un trigger UPDATE OF se déclenche dès que la colonne figure dans le SET,
-- même sans changement de valeur : seule une clause WHEN filtre vraiment.
--
-- Une clause WHEN qui lit OLD ne peut pas porter sur INSERT, ni NEW sur
-- DELETE : chaque trigger est donc scindé en deux, la création et la
-- suppression d'un côté, la modification effective de l'autre.
--
-- Deux formes de comparaison. Là où le trigger suivait toute la ligne, la
-- ligne entière est comparée hors horodatages techniques : une colonne ajoutée
-- plus tard reste visible par défaut. Là où il suivait une liste de colonnes,
-- la même liste est comparée valeur à valeur.
--
-- cairnops_targets reste tel quel : la fusion de Ressources le touche par
-- « SET updated_at = now() » précisément pour annoncer target.changed.

-- Incidents, Atteintes et Preuves ----------------------------------------------

DROP TRIGGER cairnops_incidents_changed ON cairnops_incidents;
CREATE TRIGGER cairnops_incidents_changed
AFTER INSERT OR DELETE ON cairnops_incidents
FOR EACH ROW EXECUTE FUNCTION cairnops_incident_changed_event();
CREATE TRIGGER cairnops_incidents_updated
AFTER UPDATE ON cairnops_incidents
FOR EACH ROW
WHEN ((to_jsonb(OLD) - 'updated_at') IS DISTINCT FROM (to_jsonb(NEW) - 'updated_at'))
EXECUTE FUNCTION cairnops_incident_changed_event();

DROP TRIGGER cairnops_incident_impacts_changed ON cairnops_incident_impacts;
CREATE TRIGGER cairnops_incident_impacts_changed
AFTER INSERT OR DELETE ON cairnops_incident_impacts
FOR EACH ROW EXECUTE FUNCTION cairnops_incident_child_changed_event();
CREATE TRIGGER cairnops_incident_impacts_updated
AFTER UPDATE ON cairnops_incident_impacts
FOR EACH ROW
WHEN ((to_jsonb(OLD) - 'updated_at') IS DISTINCT FROM (to_jsonb(NEW) - 'updated_at'))
EXECUTE FUNCTION cairnops_incident_child_changed_event();

-- last_seen_at avance à chaque cycle qui revoit la Preuve : c'est un battement,
-- pas un changement. Les clients relisent les Incidents à intervalle régulier,
-- ce qui suffit à garder sa fraîcheur à jour.
DROP TRIGGER cairnops_incident_evidence_changed ON cairnops_incident_evidence;
CREATE TRIGGER cairnops_incident_evidence_changed
AFTER INSERT OR DELETE ON cairnops_incident_evidence
FOR EACH ROW EXECUTE FUNCTION cairnops_incident_child_changed_event();
CREATE TRIGGER cairnops_incident_evidence_updated
AFTER UPDATE ON cairnops_incident_evidence
FOR EACH ROW
WHEN ((to_jsonb(OLD) - ARRAY['last_seen_at', 'updated_at'])
    IS DISTINCT FROM (to_jsonb(NEW) - ARRAY['last_seen_at', 'updated_at']))
EXECUTE FUNCTION cairnops_incident_child_changed_event();

-- Connecteurs ------------------------------------------------------------------

-- CompleteConnectorSync remet status = 'connected' et last_error = '' à chaque
-- cycle ; le bail, les échéances et last_synced_at restent hors de la liste,
-- comme avant.
DROP TRIGGER cairnops_connectors_changed ON cairnops_connectors;
CREATE TRIGGER cairnops_connectors_changed
AFTER INSERT OR DELETE ON cairnops_connectors
FOR EACH ROW EXECUTE FUNCTION cairnops_connector_changed_event();
CREATE TRIGGER cairnops_connectors_updated
AFTER UPDATE OF name, status, last_error, remote_version,
    compatibility, encrypted_transport, sync_interval_seconds,
    credential_management, managed_credential_id
ON cairnops_connectors
FOR EACH ROW
WHEN ((OLD.name, OLD.status, OLD.last_error, OLD.remote_version,
       OLD.compatibility, OLD.encrypted_transport, OLD.sync_interval_seconds,
       OLD.credential_management, OLD.managed_credential_id)
    IS DISTINCT FROM
      (NEW.name, NEW.status, NEW.last_error, NEW.remote_version,
       NEW.compatibility, NEW.encrypted_transport, NEW.sync_interval_seconds,
       NEW.credential_management, NEW.managed_credential_id))
EXECUTE FUNCTION cairnops_connector_changed_event();

-- Argus et Proxmox VE réécrivent le nom et les métadonnées de chaque liaison à
-- chaque cycle.
DROP TRIGGER cairnops_connector_bindings_changed ON cairnops_connector_bindings;
CREATE TRIGGER cairnops_connector_bindings_changed
AFTER INSERT OR DELETE ON cairnops_connector_bindings
FOR EACH ROW EXECUTE FUNCTION cairnops_connector_binding_changed_event();
CREATE TRIGGER cairnops_connector_bindings_updated
AFTER UPDATE ON cairnops_connector_bindings
FOR EACH ROW
WHEN ((to_jsonb(OLD) - 'updated_at') IS DISTINCT FROM (to_jsonb(NEW) - 'updated_at'))
EXECUTE FUNCTION cairnops_connector_binding_changed_event();

-- Le nom d'une Source d'Intégration suit celui de sa liaison à chaque cycle.
DROP TRIGGER cairnops_sources_configured ON cairnops_signal_sources;
CREATE TRIGGER cairnops_sources_configured
AFTER UPDATE OF name, enabled, interval_seconds, timeout_milliseconds, config,
    failure_threshold, recovery_threshold, severity
ON cairnops_signal_sources
FOR EACH ROW
WHEN ((OLD.name, OLD.enabled, OLD.interval_seconds, OLD.timeout_milliseconds,
       OLD.config, OLD.failure_threshold, OLD.recovery_threshold, OLD.severity)
    IS DISTINCT FROM
      (NEW.name, NEW.enabled, NEW.interval_seconds, NEW.timeout_milliseconds,
       NEW.config, NEW.failure_threshold, NEW.recovery_threshold, NEW.severity))
EXECUTE FUNCTION cairnops_source_changed_event();

-- Doublons possibles -----------------------------------------------------------

-- Le détecteur reconduit chaque suggestion à chaque passage.
DROP TRIGGER cairnops_reconciliation_suggestions_changed
    ON cairnops_target_reconciliation_suggestions;
CREATE TRIGGER cairnops_reconciliation_suggestions_changed
AFTER INSERT OR DELETE ON cairnops_target_reconciliation_suggestions
FOR EACH ROW EXECUTE FUNCTION cairnops_reconciliation_changed_event();
CREATE TRIGGER cairnops_reconciliation_suggestions_updated
AFTER UPDATE OF status, confidence, score, evidence, snoozed_until
ON cairnops_target_reconciliation_suggestions
FOR EACH ROW
WHEN ((OLD.status, OLD.confidence, OLD.score, OLD.evidence, OLD.snoozed_until)
    IS DISTINCT FROM
      (NEW.status, NEW.confidence, NEW.score, NEW.evidence, NEW.snoozed_until))
EXECUTE FUNCTION cairnops_reconciliation_changed_event();

-- Appareils et Indicateurs -------------------------------------------------------

DROP TRIGGER cairnops_devices_changed ON cairnops_devices;
CREATE TRIGGER cairnops_devices_changed
AFTER INSERT OR DELETE ON cairnops_devices
FOR EACH ROW EXECUTE FUNCTION cairnops_device_changed_event();
CREATE TRIGGER cairnops_devices_updated
AFTER UPDATE OF name, app_version, locale, notification_content,
    push_recipient_sealed, push_disabled_at, last_seen_at, revoked_at
ON cairnops_devices
FOR EACH ROW
WHEN ((OLD.name, OLD.app_version, OLD.locale, OLD.notification_content,
       OLD.push_recipient_sealed, OLD.push_disabled_at, OLD.last_seen_at, OLD.revoked_at)
    IS DISTINCT FROM
      (NEW.name, NEW.app_version, NEW.locale, NEW.notification_content,
       NEW.push_recipient_sealed, NEW.push_disabled_at, NEW.last_seen_at, NEW.revoked_at))
EXECUTE FUNCTION cairnops_device_changed_event();

DROP TRIGGER cairnops_context_indicators_changed ON cairnops_context_indicators;
CREATE TRIGGER cairnops_context_indicators_changed
AFTER INSERT OR DELETE ON cairnops_context_indicators
FOR EACH ROW EXECUTE FUNCTION cairnops_indicator_changed_event();
CREATE TRIGGER cairnops_context_indicators_updated
AFTER UPDATE OF enabled, label, last_value, last_observed_at, last_error
ON cairnops_context_indicators
FOR EACH ROW
WHEN ((OLD.enabled, OLD.label, OLD.last_value, OLD.last_observed_at, OLD.last_error)
    IS DISTINCT FROM
      (NEW.enabled, NEW.label, NEW.last_value, NEW.last_observed_at, NEW.last_error))
EXECUTE FUNCTION cairnops_indicator_changed_event();

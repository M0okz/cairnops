-- Vider le volet ne doit pas rompre le contrat « Résolution aux mêmes
-- destinataires ». L'entrée reste donc une mémoire de routage, mais elle ne
-- participe plus à la boîte visible ni à son compteur.
ALTER TABLE cairnops_notification_inbox
    ADD COLUMN dismissed_at timestamptz;

DROP INDEX cairnops_notification_inbox_unread_idx;
CREATE INDEX cairnops_notification_inbox_unread_idx
    ON cairnops_notification_inbox (user_id, occurred_at DESC)
    WHERE read_at IS NULL AND dismissed_at IS NULL;

DROP INDEX cairnops_notification_inbox_recent_idx;
CREATE INDEX cairnops_notification_inbox_recent_idx
    ON cairnops_notification_inbox (user_id, occurred_at DESC)
    WHERE dismissed_at IS NULL;

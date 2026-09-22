-- Le curseur d'historique suit l'instant de résolution et l'identifiant,
-- indépendamment de l'heure d'ouverture ou du nombre d'Atteintes.
CREATE INDEX cairnops_incidents_resolved_page_idx
    ON cairnops_incidents (resolved_at DESC, id DESC)
    WHERE status = 'resolved';

-- Un compte se désactive, il ne s'efface pas : toutes les traces d'une décision
-- — acquittements, invalidations, Journal d'activité — pointent vers son auteur
-- avec ON DELETE SET NULL, si bien qu'une suppression rendrait anonyme un passé
-- sur lequel des gens se sont appuyés.
ALTER TABLE cairnops_users ADD COLUMN deactivated_at timestamptz;

-- L'invariant que le code défend en transaction : au moins un Administrateur
-- actif à tout instant. L'index sert la vérification autant que la liste.
CREATE INDEX cairnops_users_active_administrators_idx
    ON cairnops_users (role)
    WHERE deactivated_at IS NULL AND role = 'administrator';

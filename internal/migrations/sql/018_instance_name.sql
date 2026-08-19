-- Une instance porte un nom, et ce nom est le sien.
--
-- Tant qu'il n'y en avait qu'une, « CairnOps » suffisait à la désigner. Dès
-- qu'on en tient deux — un banc d'essai et la production, un site et un autre —
-- rien dans l'écran ne dit laquelle on regarde : même rail, même barre, même
-- onglet. Le nom se pose à la mise en service, avec le premier Administrateur,
-- parce que c'est le seul moment où l'on sait déjà ce que cette instance sera.
--
-- La colonne naît vide pour les installations déjà en service : elles n'ont
-- jamais eu l'occasion de se nommer, et l'écran continue de dire « CairnOps »
-- jusqu'à ce qu'un Administrateur les nomme depuis les Réglages.
ALTER TABLE cairnops_installation
    ADD COLUMN name text NOT NULL DEFAULT ''
    CHECK (length(btrim(name)) <= 80 AND name = btrim(name));

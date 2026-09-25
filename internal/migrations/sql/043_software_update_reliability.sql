-- Les comparaisons sans cible plus récente, les notes absentes et les limites
-- de débit ont désormais des états stables : reclasser immédiatement les
-- services restés en reprise horaire avec l'ancienne règle.
UPDATE cairnops_software_services SET next_check_at=now() WHERE state='retry';

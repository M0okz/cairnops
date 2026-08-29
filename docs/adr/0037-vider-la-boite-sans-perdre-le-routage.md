---
status: accepted
---

# Vider la boîte sans perdre le routage des Résolutions

La boîte intégrée rend les cinquante notifications les plus récentes, mais son
compteur porte volontairement sur toutes les entrées non lues. Sans geste de
retrait, une succession d'Incidents finit donc par laisser un volet toujours
plein où une nouvelle alerte se distingue mal du bruit déjà consulté.

Chaque personne peut désormais vider sa propre boîte. Le geste retire toutes
ses entrées du volet, y compris celles qui dépassent la page rendue, et remet le
compteur à zéro. Il est distinct de la lecture : ouvrir continue de marquer lu,
tandis que vider demande une action explicite et laisse le Journal d'activité
des Incidents intact.

Une ouverture retirée du volet reste toutefois une mémoire interne de
livraison. La Résolution doit revenir aux mêmes destinataires, conformément à
l'[ADR 0033](0033-notifier-dans-l-instance-comme-ailleurs.md), même si l'un
d'eux a vidé sa boîte entre-temps. L'entrée porte donc une date de retrait que
les lectures et compteurs excluent, sans être supprimée du chemin de routage.

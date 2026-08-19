---
status: accepted
---

# Consolider progressivement les données de supervision

CairnOps conserve par défaut les Observations brutes pendant 30 jours, les agrégats horaires de disponibilité et de latence pendant 13 mois, et les agrégats journaliers sans limite ; les Incidents et leur journal humain sont conservés sans limite. Ces durées restent configurables par un Administrateur, ce qui préserve l'analyse récente détaillée et l'historique à long terme sans faire croître PostgreSQL au rythme de chaque contrôle brut.

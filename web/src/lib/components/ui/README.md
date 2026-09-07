# Composants shadcn-svelte

`button`, `input`, `toggle` et `toggle-group` sont copiés du registre officiel
shadcn-svelte, sous licence MIT (voir LICENSE.md). Les URLs et empreintes des
réponses du registre figurent dans shadcn-sources.json.

Adaptations locales : alias `$lib`, conteneurs de portée `shadcn-control` dans les consommateurs pour isoler
Tailwind sans reset global, contexte ToggleGroup réactif et variable d’espacement
appliquée via CSSOM pour préserver la CSP. Les variantes et classes visuelles du
registre sont conservées. SegmentedControl adapte uniquement les libellés,
compteurs et la sélection obligatoire de CairnOps.

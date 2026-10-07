# Composants shadcn-svelte

`input`, `toggle` et `toggle-group` sont copiés du registre officiel
shadcn-svelte, sous licence MIT (voir LICENSE.md). Les URLs et empreintes des
réponses du registre figurent dans shadcn-sources.json.

Adaptations locales : alias `$lib`, conteneurs de portée `shadcn-control` dans les consommateurs pour isoler
Tailwind sans reset global, contexte ToggleGroup réactif et variable d’espacement
appliquée via CSSOM pour préserver la CSP. Les variantes et classes visuelles du
registre sont conservées. SegmentedControl adapte uniquement les libellés,
compteurs et la sélection obligatoire de CairnOps.

# Primitives Bits UI habillées par CairnOps

`Checkbox`, `Switch` et `Modal` s'appuient directement sur Bits UI, sans
Tailwind : leur dessin vient des jetons de `app.css`.

`Button` est le bouton de l'application. Ses variantes (`primary`, `danger`,
`quiet`, `close`, taille `sm`) reprennent les classes `.btn` et `.close` de
`app.css` ; le Button du registre shadcn, dessiné en utilitaires Tailwind,
n'est plus copié. Une classe passée à `Button` ne porte pas la portée CSS de
l'appelant : une règle qui la vise s'écrit `.ancetre :global(.classe)`.

`Modal` est la seule fenêtre modale de l'application, volets de détail compris
(`side`). Le parent la monte et la retire ; la boîte reste dessinée par
l'appelant, qui y étale les attributs reçus pour garder ses styles scopés.
Le verrou de défilement de Bits est remplacé par un verrou CSSOM : celui de
Bits restaure le style du body par `setAttribute('style')`, refusé par la CSP.

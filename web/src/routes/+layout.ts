/* Application monopage : le handler Go renvoie index.html pour toute route sans
 * extension, ce qui permet des routes dynamiques comme /cibles/[id] sans
 * pré-rendu. */
export const prerender = false;
export const ssr = false;

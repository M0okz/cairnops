<script module lang="ts">
  /* Marques des produits tiers.
   *
   * Zabbix et Uptime Kuma viennent de la collection selfh.st/icons, qui publie des
   * marques carrées normalisées pour les logiciels auto-hébergés :
   *   https://cdn.jsdelivr.net/gh/selfhst/icons@main/svg/zabbix.svg
   *   https://cdn.jsdelivr.net/gh/selfhst/icons@main/svg/uptime-kuma.svg
   * PatchMon et Argus viennent directement de leurs dépôts officiels :
   *   https://github.com/PatchMon/PatchMon/blob/main/frontend/public/assets/logo_square_default.svg
   *   https://github.com/release-argus/Argus/blob/master/web/ui/react-app/public/favicon.svg
   *
   * Zabbix ne publie qu'un logotype horizontal, illisible dans une pastille ;
   * cette collection en fournit le symbole carré. Prendre les deux à la même
   * source garantit le même canevas et le même traitement.
   *
   * Zabbix® est une marque de Zabbix LLC, Uptime Kuma est sous licence MIT et
   * PatchMon est distribué sous AGPL-3.0.
   * Usage nominatif : ces marques désignent le produit auquel CairnOps se
   * connecte, rien d'autre. Aucun fichier n'est modifié.
   *
   * Intégrés en <img> data URI plutôt qu'en SVG inline : les fichiers
   * déclarent des <defs> dont les identifiants entreraient en collision s'ils
   * apparaissaient plusieurs fois, et la politique de sécurité de l'instance
   * autorise déjà `img-src 'self' data:`. */

  export type BrandName = 'zabbix' | 'uptime_kuma' | 'patchmon' | 'argus';

  const marks: Record<BrandName, { src: string; label: string }> = {
    zabbix: {
      src: 'data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHhtbDpzcGFjZT0icHJlc2VydmUiIHZpZXdCb3g9IjAgMCA1MTIgNTEyIj48cGF0aCBkPSJNMzg0LjQgNTEySDEyNy42QzU3LjEgNTEyIDAgNDU0LjkgMCAzODQuNFYxMjcuNkMwIDU3LjEgNTcuMSAwIDEyNy42IDBoMjU2LjhDNDU0LjkgMCA1MTIgNTcuMSA1MTIgMTI3LjZ2MjU2LjhjMCA3MC41LTU3LjEgMTI3LjYtMTI3LjYgMTI3LjYiIHN0eWxlPSJmaWxsOiNjYjI0MTUiLz48cGF0aCBkPSJNMTMxLjEgOTguMXYzOS40aDE5MS4yTDEyNi4yIDM3OS4xdjM0LjhoMjU5LjZ2LTM5LjRIMTg1bDE5NS45LTI0MS42Vjk4LjF6IiBzdHlsZT0iZmlsbDojZmZmIi8+PC9zdmc+',
      label: 'Zabbix'
    },
    uptime_kuma: {
      src: 'data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHhtbDpzcGFjZT0icHJlc2VydmUiIHZpZXdCb3g9IjAgMCA2MjIgNjIyIj48bGluZWFyR3JhZGllbnQgaWQ9InVwdGltZS1rdW1hX3N2Z19fYSIgeDE9Ii04Mi40IiB4Mj0iMTIxLjY3IiB5MT0iNzUzLjkxOSIgeTI9Ijk0OS4yNTkiIGdyYWRpZW50VHJhbnNmb3JtPSJ0cmFuc2xhdGUoMzIwLjAwMSAtNDg4KSIgZ3JhZGllbnRVbml0cz0idXNlclNwYWNlT25Vc2UiPjxzdG9wIG9mZnNldD0iMCIgc3R5bGU9InN0b3AtY29sb3I6IzVjZGQ4YiIvPjxzdG9wIG9mZnNldD0iMSIgc3R5bGU9InN0b3AtY29sb3I6Izg2ZTZhOSIvPjwvbGluZWFyR3JhZGllbnQ+PHBhdGggZD0iTTQ4MS40IDIyNi42YzUzLjcgMTIyLjcgNTMuNyAxOTkuNyAwIDIzMC45LTgwLjUgNDYuNy0yOTAuNCA2MS0zNTAuOS0xMC45LTQwLjMtNDcuOS00MC4zLTEyMS4yIDAtMjIwIDQxLTY3LjUgOTkuMi0xMDEuMiAxNzQuNi0xMDEuMiA3NS41IDAgMTM0LjMgMzMuOCAxNzYuMyAxMDEuMnoiIHN0eWxlPSJmaWxsOnVybCgjdXB0aW1lLWt1bWFfc3ZnX19hKTtzdHJva2U6I2YyZjJmMjtzdHJva2Utd2lkdGg6MjAwO3N0cm9rZS1vcGFjaXR5Oi41MSIvPjwvc3ZnPg==',
      label: 'Uptime Kuma'
    },
    patchmon: {
      src: 'data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHhtbG5zOnhsaW5rPSJodHRwOi8vd3d3LnczLm9yZy8xOTk5L3hsaW5rIiB3aWR0aD0iNTAwIiB6b29tQW5kUGFuPSJtYWduaWZ5IiB2aWV3Qm94PSIwIDAgMzc1IDM3NC45OTk5OTEiIGhlaWdodD0iNTAwIiBwcmVzZXJ2ZUFzcGVjdFJhdGlvPSJ4TWlkWU1pZCBtZWV0IiB2ZXJzaW9uPSIxLjAiPjxkZWZzPjxnLz48Y2xpcFBhdGggaWQ9ImQ2MjYzMmQ0MTMiPjxwYXRoIGQ9Ik0gMjkgMjggTCAzMDQgMjggTCAzMDQgMzUwIEwgMjkgMzUwIFogTSAyOSAyOCAiIGNsaXAtcnVsZT0ibm9uemVybyIvPjwvY2xpcFBhdGg+PGNsaXBQYXRoIGlkPSJlY2M4YjRkOGVkIj48cGF0aCBkPSJNIDE4Ny40OTYwOTQgLTM5Ljk5NjA5NCBMIDQxNi42MDE1NjIgMTg5LjEwNTQ2OSBMIDE4Ny40OTYwOTQgNDE4LjIwNzAzMSBMIC00MS42MDU0NjkgMTg5LjEwNTQ2OSBaIE0gMTg3LjQ5NjA5NCAtMzkuOTk2MDk0ICIgY2xpcC1ydWxlPSJub256ZXJvIi8+PC9jbGlwUGF0aD48Y2xpcFBhdGggaWQ9IjMwMTZkYjk0MmYiPjxwYXRoIGQ9Ik0gMTg3LjQ5NjA5NCAtMzkuOTk2MDk0IEwgNDE2LjYwMTU2MiAxODkuMTA1NDY5IEwgMTg3LjQ5NjA5NCA0MTguMjA3MDMxIEwgLTQxLjYwNTQ2OSAxODkuMTA1NDY5IFogTSAxODcuNDk2MDk0IC0zOS45OTYwOTQgIiBjbGlwLXJ1bGU9Im5vbnplcm8iLz48L2NsaXBQYXRoPjxjbGlwUGF0aCBpZD0iMDI5ZjhhZTZhOCI+PHBhdGggZD0iTSAyOSAyOCBMIDMwNCAyOCBMIDMwNCAzNTAgTCAyOSAzNTAgWiBNIDI5IDI4ICIgY2xpcC1ydWxlPSJub256ZXJvIi8+PC9jbGlwUGF0aD48Y2xpcFBhdGggaWQ9IjJkMzc0YjVlNzYiPjxwYXRoIGQ9Ik0gMTg3LjQ5NjA5NCAtMzkuOTk2MDk0IEwgNDE2LjYwMTU2MiAxODkuMTA1NDY5IEwgMTg3LjQ5NjA5NCA0MTguMjA3MDMxIEwgLTQxLjYwNTQ2OSAxODkuMTA1NDY5IFogTSAxODcuNDk2MDk0IC0zOS45OTYwOTQgIiBjbGlwLXJ1bGU9Im5vbnplcm8iLz48L2NsaXBQYXRoPjxjbGlwUGF0aCBpZD0iNTQ0ZDgyMzYwNiI+PHBhdGggZD0iTSAxODcuNDk2MDk0IC0zOS45OTYwOTQgTCA0MTYuNjAxNTYyIDE4OS4xMDU0NjkgTCAxODcuNDk2MDk0IDQxOC4yMDcwMzEgTCAtNDEuNjA1NDY5IDE4OS4xMDU0NjkgWiBNIDE4Ny40OTYwOTQgLTM5Ljk5NjA5NCAiIGNsaXAtcnVsZT0ibm9uemVybyIvPjwvY2xpcFBhdGg+PGNsaXBQYXRoIGlkPSJiODhhMjc2MTE2Ij48cGF0aCBkPSJNIDE4Ny40OTYwOTQgLTM5Ljk5NjA5NCBMIDQxNi42MDE1NjIgMTg5LjEwNTQ2OSBMIDE4Ny40OTYwOTQgNDE4LjIwNzAzMSBMIC00MS42MDU0NjkgMTg5LjEwNTQ2OSBaIE0gMTg3LjQ5NjA5NCAtMzkuOTk2MDk0ICIgY2xpcC1ydWxlPSJub256ZXJvIi8+PC9jbGlwUGF0aD48Y2xpcFBhdGggaWQ9Ijk4YzI2ZTExYTQiPjxyZWN0IHg9IjAiIHdpZHRoPSIxMDMiIHk9IjAiIGhlaWdodD0iMjA4Ii8+PC9jbGlwUGF0aD48L2RlZnM+PGcgY2xpcC1wYXRoPSJ1cmwoI2Q2MjYzMmQ0MTMpIj48ZyBjbGlwLXBhdGg9InVybCgjZWNjOGI0ZDhlZCkiPjxnIGNsaXAtcGF0aD0idXJsKCMzMDE2ZGI5NDJmKSI+PHBhdGggZmlsbD0iI2ZmNzUxZiIgZD0iTSAzMDMuMjE0ODQ0IDMwMi43NjE3MTkgQyAyODAuNzY1NjI1IDMyNS4yMTQ4NDQgMjUyLjE2MDE1NiAzNDAuNTAzOTA2IDIyMS4wMTU2MjUgMzQ2LjY5OTIxOSBDIDE4OS44NzUgMzUyLjg5MDYyNSAxNTcuNTkzNzUgMzQ5LjcxNDg0NCAxMjguMjYxNzE5IDMzNy41NjI1IEMgOTguOTI1NzgxIDMyNS40MTAxNTYgNzMuODUxNTYyIDMwNC44MzU5MzggNTYuMjEwOTM4IDI3OC40MzM1OTQgQyAzOC41NzAzMTIgMjUyLjAzMTI1IDI5LjE1NjI1IDIyMC45OTIxODggMjkuMTU2MjUgMTg5LjI0MjE4OCBDIDI5LjE1NjI1IDE1Ny40ODgyODEgMzguNTcwMzEyIDEyNi40NDkyMTkgNTYuMjEwOTM4IDEwMC4wNTA3ODEgQyA3My44NTE1NjIgNzMuNjQ4NDM4IDk4LjkyNTc4MSA1My4wNzAzMTIgMTI4LjI2MTcxOSA0MC45MjE4NzUgQyAxNTcuNTkzNzUgMjguNzY5NTMxIDE4OS44NzUgMjUuNTg5ODQ0IDIyMS4wMTU2MjUgMzEuNzg1MTU2IEMgMjUyLjE2MDE1NiAzNy45ODA0NjkgMjgwLjc2NTYyNSA1My4yNjk1MzEgMzAzLjIxNDg0NCA3NS43MjI2NTYgTCAxODkuNjk1MzEyIDE4OS4yNDIxODggWiBNIDMwMy4yMTQ4NDQgMzAyLjc2MTcxOSAiIGZpbGwtb3BhY2l0eT0iMSIgZmlsbC1ydWxlPSJub256ZXJvIi8+PC9nPjwvZz48L2c+PGcgY2xpcC1wYXRoPSJ1cmwoIzAyOWY4YWU2YTgpIj48ZyBjbGlwLXBhdGg9InVybCgjMmQzNzRiNWU3NikiPjxnIGNsaXAtcGF0aD0idXJsKCM1NDRkODIzNjA2KSI+PGcgY2xpcC1wYXRoPSJ1cmwoI2I4OGEyNzYxMTYpIj48cGF0aCBmaWxsPSIjNjFiMzNhIiBkPSJNIDMwMy4xNDQ1MzEgMzAyLjU1MDc4MSBDIDI4MC43MDcwMzEgMzI0Ljk4ODI4MSAyNTIuMTE3MTg4IDM0MC4yNjk1MzEgMjIwLjk5NjA5NCAzNDYuNDYwOTM4IEMgMTg5Ljg3NSAzNTIuNjUyMzQ0IDE1Ny42MTMyODEgMzQ5LjQ3MjY1NiAxMjguMjk2ODc1IDMzNy4zMzIwMzEgQyA5OC45ODA0NjkgMzI1LjE4NzUgNzMuOTIxODc1IDMwNC42MjEwOTQgNTYuMjkyOTY5IDI3OC4yMzgyODEgQyAzOC42NjQwNjIgMjUxLjg1MTU2MiAyOS4yNTM5MDYgMjIwLjgzMjAzMSAyOS4yNTM5MDYgMTg5LjEwMTU2MiBDIDI5LjI1MzkwNiAxNTcuMzY3MTg4IDM4LjY2NDA2MiAxMjYuMzQ3NjU2IDU2LjI5Mjk2OSA5OS45NjQ4NDQgQyA3My45MjE4NzUgNzMuNTc4MTI1IDk4Ljk4MDQ2OSA1My4wMTU2MjUgMTI4LjI5Njg3NSA0MC44NzEwOTQgQyAxNTcuNjEzMjgxIDI4LjcyNjU2MiAxODkuODc1IDI1LjU1MDc4MSAyMjAuOTk2MDk0IDMxLjc0MjE4OCBDIDI1Mi4xMTcxODggMzcuOTI5Njg4IDI4MC43MDcwMzEgNTMuMjEwOTM4IDMwMy4xNDQ1MzEgNzUuNjUyMzQ0IEwgMTg5LjY5NTMxMiAxODkuMTAxNTYyIFogTSAzMDMuMTQ0NTMxIDMwMi41NTA3ODEgIiBmaWxsLW9wYWNpdHk9IjEiIGZpbGwtcnVsZT0ibm9uemVybyIvPjwvZz48L2c+PC9nPjwvZz48ZyB0cmFuc2Zvcm09Im1hdHJpeCgxLCAwLCAwLCAxLCAxMzYsIDApIj48ZyBjbGlwLXBhdGg9InVybCgjOThjMjZlMTFhNCkiPjxnIGZpbGw9IiNmZjc1MWYiIGZpbGwtb3BhY2l0eT0iMSI+PGcgdHJhbnNmb3JtPSJ0cmFuc2xhdGUoMC40NTcxNjQsIDExNi40MDM1NDMpIj48Zz48cGF0aCBkPSJNIDE5LjczNDM3NSAtMTguNzE4NzUgQyAxOS43MzQzNzUgLTIxLjY2NDA2MiAyMC4wMTU2MjUgLTI0LjQ0MTQwNiAyMC41NzgxMjUgLTI3LjA0Njg3NSBDIDIxLjE0ODQzOCAtMjkuNjYwMTU2IDIyLjA2MjUgLTMyLjIxMDkzOCAyMy4zMTI1IC0zNC43MDMxMjUgQyAyNC41NjI1IC0zNy4yMDMxMjUgMjYuMjA3MDMxIC0zOS4zNTkzNzUgMjguMjUgLTQxLjE3MTg3NSBDIDMzLjY4NzUgLTQ3LjA2NjQwNiA0MS4yODUxNTYgLTUwLjAxNTYyNSA1MS4wNDY4NzUgLTUwLjAxNTYyNSBDIDU5LjIxMDkzOCAtNTAuMDE1NjI1IDY2LjQ2ODc1IC00Ni45NTMxMjUgNzIuODEyNSAtNDAuODI4MTI1IEMgNzkuMTY0MDYyIC0zNC43MDMxMjUgODIuMzQzNzUgLTI3LjMzMjAzMSA4Mi4zNDM3NSAtMTguNzE4NzUgQyA4Mi4zNDM3NSAtOS40MTQwNjIgNzkuMjgxMjUgLTEuOTI1NzgxIDczLjE1NjI1IDMuNzUgQyA2Ny4yNTc4MTIgOS42NDQ1MzEgNTkuODkwNjI1IDEyLjU5Mzc1IDUxLjA0Njg3NSAxMi41OTM3NSBDIDQyLjY0ODQzOCAxMi41OTM3NSAzNS4zMzIwMzEgOS40NzI2NTYgMjkuMDkzNzUgMy4yMzQzNzUgQyAyMi44NTE1NjIgLTMuMDAzOTA2IDE5LjczNDM3NSAtMTAuMzIwMzEyIDE5LjczNDM3NSAtMTguNzE4NzUgWiBNIDE5LjczNDM3NSAtMTguNzE4NzUgIi8+PC9nPjwvZz48L2c+PC9nPjwvZz48L3N2Zz4=',
      label: 'PatchMon'
    },
    argus: {
      src: 'data:image/svg+xml,%3Csvg%20xmlns=%22http://www.w3.org/2000/svg%22%20viewBox=%220%200%2067.733%2067.733%22%3E%3Crect%20fill=%22%230082c7%22%20width=%227.236%22%20height=%2216.961%22%20x=%2237.662%22%20y=%2250.773%22%20rx=%222.979%22/%3E%3Crect%20fill=%22%230082c7%22%20width=%227.236%22%20height=%2216.961%22%20x=%2222.836%22%20y=%2250.773%22%20rx=%222.979%22/%3E%3Cellipse%20fill=%22%230eabff%22%20cx=%2233.867%22%20cy=%2229.257%22%20rx=%2229.369%22%20ry=%2229.257%22/%3E%3Cellipse%20fill=%22%230081c7%22%20cx=%2233.867%22%20cy=%2229.257%22%20rx=%2227.959%22%20ry=%2219.222%22/%3E%3Cellipse%20fill=%22white%22%20cx=%2233.867%22%20cy=%2229.257%22%20rx=%2226.035%22%20ry=%2215.052%22/%3E%3Cellipse%20fill=%22%23004970%22%20cx=%2233.867%22%20cy=%2229.257%22%20rx=%224.824%22%20ry=%224.805%22/%3E%3C/svg%3E',
      label: 'Argus'
    }
  };
</script>

<script lang="ts">
  let { name, size = 38 }: { name: BrandName; size?: number } = $props();

  const mark = $derived(marks[name]);
</script>

<span class="brand" style:--mark={`${size}px`}>
  <img src={mark.src} alt={mark.label} />
</span>

<style>
  .brand {
    height: var(--mark);
    width: var(--mark);
    flex: none;
    display: grid;
    place-items: center;
    padding: 0.1875rem;
    border-radius: var(--r-m);
    background: var(--surface-2);
    overflow: hidden;
  }

  img {
    display: block;
    max-width: 100%;
    max-height: 100%;
    width: auto;
    height: auto;
    object-fit: contain;
  }
</style>

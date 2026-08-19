<script module lang="ts">
  /* Marques des produits tiers.
   *
   * Les deux fichiers viennent de la collection selfh.st/icons, qui publie des
   * marques carrées normalisées pour les logiciels auto-hébergés :
   *   https://cdn.jsdelivr.net/gh/selfhst/icons@main/svg/zabbix.svg
   *   https://cdn.jsdelivr.net/gh/selfhst/icons@main/svg/uptime-kuma.svg
   *
   * Zabbix ne publie qu'un logotype horizontal, illisible dans une pastille ;
   * cette collection en fournit le symbole carré. Prendre les deux à la même
   * source garantit le même canevas et le même traitement.
   *
   * Zabbix® est une marque de Zabbix LLC, Uptime Kuma est sous licence MIT.
   * Usage nominatif : ces marques désignent le produit auquel CairnOps se
   * connecte, rien d'autre. Aucun fichier n'est modifié.
   *
   * Intégrés en <img> data URI plutôt qu'en SVG inline : les fichiers
   * déclarent des <defs> dont les identifiants entreraient en collision s'ils
   * apparaissaient plusieurs fois, et la politique de sécurité de l'instance
   * autorise déjà `img-src 'self' data:`. */

  export type BrandName = 'zabbix' | 'uptime_kuma';

  const marks: Record<BrandName, { src: string; label: string }> = {
    zabbix: {
      src: 'data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHhtbDpzcGFjZT0icHJlc2VydmUiIHZpZXdCb3g9IjAgMCA1MTIgNTEyIj48cGF0aCBkPSJNMzg0LjQgNTEySDEyNy42QzU3LjEgNTEyIDAgNDU0LjkgMCAzODQuNFYxMjcuNkMwIDU3LjEgNTcuMSAwIDEyNy42IDBoMjU2LjhDNDU0LjkgMCA1MTIgNTcuMSA1MTIgMTI3LjZ2MjU2LjhjMCA3MC41LTU3LjEgMTI3LjYtMTI3LjYgMTI3LjYiIHN0eWxlPSJmaWxsOiNjYjI0MTUiLz48cGF0aCBkPSJNMTMxLjEgOTguMXYzOS40aDE5MS4yTDEyNi4yIDM3OS4xdjM0LjhoMjU5LjZ2LTM5LjRIMTg1bDE5NS45LTI0MS42Vjk4LjF6IiBzdHlsZT0iZmlsbDojZmZmIi8+PC9zdmc+',
      label: 'Zabbix'
    },
    uptime_kuma: {
      src: 'data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHhtbDpzcGFjZT0icHJlc2VydmUiIHZpZXdCb3g9IjAgMCA2MjIgNjIyIj48bGluZWFyR3JhZGllbnQgaWQ9InVwdGltZS1rdW1hX3N2Z19fYSIgeDE9Ii04Mi40IiB4Mj0iMTIxLjY3IiB5MT0iNzUzLjkxOSIgeTI9Ijk0OS4yNTkiIGdyYWRpZW50VHJhbnNmb3JtPSJ0cmFuc2xhdGUoMzIwLjAwMSAtNDg4KSIgZ3JhZGllbnRVbml0cz0idXNlclNwYWNlT25Vc2UiPjxzdG9wIG9mZnNldD0iMCIgc3R5bGU9InN0b3AtY29sb3I6IzVjZGQ4YiIvPjxzdG9wIG9mZnNldD0iMSIgc3R5bGU9InN0b3AtY29sb3I6Izg2ZTZhOSIvPjwvbGluZWFyR3JhZGllbnQ+PHBhdGggZD0iTTQ4MS40IDIyNi42YzUzLjcgMTIyLjcgNTMuNyAxOTkuNyAwIDIzMC45LTgwLjUgNDYuNy0yOTAuNCA2MS0zNTAuOS0xMC45LTQwLjMtNDcuOS00MC4zLTEyMS4yIDAtMjIwIDQxLTY3LjUgOTkuMi0xMDEuMiAxNzQuNi0xMDEuMiA3NS41IDAgMTM0LjMgMzMuOCAxNzYuMyAxMDEuMnoiIHN0eWxlPSJmaWxsOnVybCgjdXB0aW1lLWt1bWFfc3ZnX19hKTtzdHJva2U6I2YyZjJmMjtzdHJva2Utd2lkdGg6MjAwO3N0cm9rZS1vcGFjaXR5Oi41MSIvPjwvc3ZnPg==',
      label: 'Uptime Kuma'
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

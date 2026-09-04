<script lang="ts">
  /* Palette — la recherche globale.
   *
   * Elle cherche un objet et y conduit ; elle ne filtre aucune liste. Les
   * écrans gardent leurs propres filtres, qui répondent à l'autre question :
   * réduire ce qui est déjà sous les yeux.
   *
   * Rien n'est demandé au serveur : la projection partagée porte déjà les
   * Cibles, les Incidents, les Maintenances et les Connecteurs. À l'échelle
   * d'un parc supervisé depuis un rail unique, la recherche tient en mémoire ;
   * le jour où elle n'y tiendra plus, c'est un endpoint qu'il faudra, pas un
   * autre composant. */

  import { goto } from '$app/navigation';
  import Icon, { type IconName } from './Icon.svelte';
  import { incidentHref } from '$lib/incident-detail';
  import { palette } from '$lib/palette.svelte';
  import { session } from '$lib/session.svelte';
  import { natureLabel, severityLabel, severityTone, since, stateLabel, stateTones, type Tone } from '$lib/format';
  import { i18n, plural, t } from '$lib/i18n.svelte';

  type Hit = {
    key: string;
    href: string;
    label: string;
    detail: string;
    badge?: string;
    tone?: Tone;
    score: number;
  };

  type Group = { key: string; label: string; icon: IconName; hits: Hit[] };

  /* Cinq par famille : au-delà, la Palette redevient une liste, et une liste se
   * consulte mieux sur son écran. */
  const PER_GROUP = 5;

  const destinations = $derived<Array<{ href: string; label: string; detail: string }>>([
    { href: '/', label: t('nav.overview'), detail: t('palette.goto.overview') },
    { href: '/cibles', label: t('nav.targets'), detail: t('palette.goto.targets') },
    { href: '/incidents', label: t('nav.incidents'), detail: t('palette.goto.incidents') },
    { href: '/maintenance', label: t('nav.maintenance'), detail: t('palette.goto.maintenance') },
    { href: '/connecteurs', label: t('nav.connectors'), detail: t('palette.goto.connectors') },
    { href: '/sante', label: t('nav.health'), detail: t('palette.goto.health') },
    { href: '/reglages', label: t('nav.settings'), detail: t('palette.goto.settings') }
  ]);

  let raw = $state('');
  let cursor = $state(0);
  let field = $state<HTMLInputElement | null>(null);
  let list = $state<HTMLDivElement | null>(null);

  /* On compare sans accents ni casse : « sante » doit trouver « Santé », et
   * personne ne tape les diacritiques dans un champ de recherche. */
  const fold = (value: string) =>
    value.normalize('NFD').replace(/\p{Diacritic}/gu, '').toLocaleLowerCase(i18n.locale);

  const query = $derived(fold(raw.trim()));

  /* Le classement tient en trois idées : le nom compte plus que le reste, un
   * début de mot compte plus qu'une occurrence au milieu, et ce qui ne
   * correspond qu'accessoirement passe après tout le reste. */
  function score(name: string, extras: string[] = []): number {
    const hay = fold(name);
    const at = hay.indexOf(query);
    if (at === 0) return 100;
    if (at > 0) return /[\s·\-_/.:]/.test(hay[at - 1]) ? 80 : 60;
    return extras.some((extra) => fold(extra).includes(query)) ? 30 : 0;
  }

  function best(hits: Hit[]): Hit[] {
    return hits
      .filter((hit) => hit.score > 0)
      .sort((a, b) => b.score - a.score || a.label.localeCompare(b.label, i18n.locale))
      .slice(0, PER_GROUP);
  }

  function incidentTargetLabel(incident: (typeof session.actionable)[number]): string {
    if (incident.affected_target_count > 1) {
      return plural('incident.targetsAffected', incident.affected_target_count);
    }
    return incident.impacts[0]?.target_name ?? t('nav.incidents');
  }

  const connectorLabels = $derived<Record<string, string>>({
    zabbix: 'Zabbix',
    uptime_kuma: 'Uptime Kuma',
    patchmon: 'PatchMon',
    argus: 'Argus',
    generic_webhook: t('connector.genericWebhook')
  });

  const groups = $derived.by<Group[]>(() => {
    /* Sans frappe, la Palette n'invente pas de pertinence : elle offre les
     * destinations, ce qui en fait aussi une navigation au clavier. */
    if (!query) {
      return [
        {
          key: 'goto',
          label: t('palette.group.goto'),
          icon: 'overview',
          hits: destinations.map((destination) => ({
            key: `goto:${destination.href}`,
            href: destination.href,
            label: destination.label,
            detail: destination.detail,
            score: 1
          }))
        }
      ];
    }

    const targets: Group = {
      key: 'targets',
      label: t('nav.targets'),
      icon: 'targets',
      hits: best(
        session.targets.map((target) => {
          const state = session.targetState(target);
          return {
            key: `target:${target.id}`,
            href: `/cibles/${target.id}`,
            label: target.name,
            detail:
              target.description ||
              plural('palette.sources', target.sources.length + target.external_source_count),
            badge: stateLabel(state),
            tone: stateTones[state],
            score: score(target.name, [
              target.description,
              ...target.aliases,
              ...target.sources.map((source) => source.name)
            ])
          };
        })
      )
    };

    /* Le détail garde l'adresse de l'Incident : la recherche ne perd ni sa
     * Nature ni sa chronologie en passant par la Cible. */
    const incidents: Group = {
      key: 'incidents',
      label: t('nav.incidents'),
      icon: 'incidents',
      hits: best(
        session.actionable.map((incident) => {
          const target = incidentTargetLabel(incident);
          return {
            key: `incident:${incident.id}`,
            href: incidentHref(incident.id),
            label: `${target} · ${natureLabel(incident)}`,
            detail: `${t('palette.openedFor', { duration: since(incident.opened_at) })} · ${
              incident.acknowledged_at ? t('incident.acknowledged') : t('incident.unacknowledged')
            }`,
            badge: severityLabel(incident.severity),
            tone: severityTone(incident.severity),
            score: score(target, [natureLabel(incident), incident.nature_label])
          };
        })
      )
    };

    const maintenances: Group = {
      key: 'maintenances',
      label: t('nav.maintenance'),
      icon: 'maintenance',
      hits: best(
        session.visibleMaintenances.map((maintenance) => ({
          key: `maintenance:${maintenance.id}`,
          href: '/maintenance',
          label: maintenance.name,
          detail: maintenance.targets.map((target) => target.name).join(', ') || maintenance.reason,
          badge:
            maintenance.state === 'active'
              ? t('maintenance.state.active')
              : t('maintenance.state.upcoming'),
          tone: maintenance.state === 'active' ? 'info' : 'idle',
          score: score(maintenance.name, [
            maintenance.reason,
            ...maintenance.targets.map((target) => target.name)
          ])
        }))
      )
    };

    const connectors: Group = {
      key: 'connectors',
      label: t('nav.connectors'),
      icon: 'connectors',
      hits: best(
        session.connectors.map((connector) => ({
          key: `connector:${connector.id}`,
          href: `/connecteurs/${connector.kind.replace('_', '-')}`,
          label: connector.name,
          detail: connector.endpoint,
          badge: connectorLabels[connector.kind] ?? connector.kind,
          score: score(connector.name, [connector.endpoint, connectorLabels[connector.kind] ?? ''])
        }))
      )
    };

    const places: Group = {
      key: 'goto',
      label: t('palette.group.goto'),
      icon: 'overview',
      hits: best(
        destinations.map((destination) => ({
          key: `goto:${destination.href}`,
          href: destination.href,
          label: destination.label,
          detail: destination.detail,
          score: score(destination.label)
        }))
      )
    };

    return [targets, incidents, maintenances, connectors, places].filter((group) => group.hits.length > 0);
  });

  /* La liste aplatie est ce que parcourent les flèches ; les familles ne sont
   * qu'une mise en forme. */
  const flat = $derived(groups.flatMap((group) => group.hits));

  $effect(() => {
    /* Toute nouvelle frappe ramène la sélection en tête : la meilleure
     * réponse est toujours celle du haut. */
    void query;
    cursor = 0;
  });

  $effect(() => {
    if (cursor > flat.length - 1) cursor = Math.max(0, flat.length - 1);
  });

  $effect(() => {
    if (!palette.open) return;
    raw = '';
    cursor = 0;
    field?.focus();
  });

  function move(step: number) {
    if (flat.length === 0) return;
    cursor = (cursor + step + flat.length) % flat.length;
    list
      ?.querySelector(`[data-index="${cursor}"]`)
      ?.scrollIntoView({ block: 'nearest' });
  }

  function choose(hit: Hit | undefined) {
    if (!hit) return;
    palette.hide();
    void goto(hit.href);
  }

  function shortcut(event: KeyboardEvent) {
    if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
      event.preventDefault();
      palette.toggle();
    }
  }

  function steer(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.preventDefault();
      palette.hide();
    } else if (event.key === 'ArrowDown') {
      event.preventDefault();
      move(1);
    } else if (event.key === 'ArrowUp') {
      event.preventDefault();
      move(-1);
    } else if (event.key === 'Enter') {
      event.preventDefault();
      choose(flat[cursor]);
    }
  }
</script>

<svelte:window onkeydown={shortcut} />

{#if palette.open}
  <!-- Le fond ferme la Palette au clic ; le clavier a déjà Échap, ce fond n'a
       donc pas à être atteignable au clavier. -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div class="scrim palette-scrim" onclick={() => palette.hide()}>
    <div
      class="palette"
      role="dialog"
      tabindex="-1"
      aria-modal="true"
      aria-label={t('palette.title')}
      onclick={(event) => event.stopPropagation()}
    >
      <div class="palette-field">
        <Icon name="search" />
        <input
          bind:this={field}
          bind:value={raw}
          type="text"
          role="combobox"
          aria-expanded="true"
          aria-controls="palette-results"
          aria-activedescendant={flat[cursor] ? `palette-hit-${cursor}` : undefined}
          aria-label={t('palette.fieldLabel')}
          placeholder={t('palette.placeholder')}
          autocomplete="off"
          spellcheck="false"
          onkeydown={steer}
        />
        <kbd>{t('key.escape')}</kbd>
      </div>

      <div class="palette-results" id="palette-results" role="listbox" bind:this={list}>
        {#each groups as group (group.key)}
          <div class="palette-group">
            <p class="palette-group-head">
              <Icon name={group.icon} size={12} />
              {group.label}
            </p>
            {#each group.hits as hit (hit.key)}
              {@const index = flat.indexOf(hit)}
              <button
                class="palette-hit"
                type="button"
                role="option"
                id="palette-hit-{index}"
                data-index={index}
                aria-selected={index === cursor}
                onmouseenter={() => (cursor = index)}
                onclick={() => choose(hit)}
              >
                <span class="palette-hit-text">
                  <strong>{hit.label}</strong>
                  {#if hit.detail}<small>{hit.detail}</small>{/if}
                </span>
                {#if hit.badge}
                  <span class="pill {hit.tone ?? ''}">{hit.badge}</span>
                {/if}
              </button>
            {/each}
          </div>
        {:else}
          <div class="palette-empty">
            <strong>{t('palette.empty', { query: raw.trim() })}</strong>
            {t('palette.emptyHint')}
          </div>
        {/each}
      </div>

      <footer class="palette-foot">
        <span><kbd>↑</kbd><kbd>↓</kbd> {t('palette.browse')}</span>
        <span><kbd>↵</kbd> {t('palette.select')}</span>
        <span><kbd>{t('key.escape')}</kbd> {t('common.close')}</span>
      </footer>
    </div>
  </div>
{/if}

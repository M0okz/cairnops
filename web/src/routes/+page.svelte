<script lang="ts">
  /* Écran 4g — Vue d'ensemble, orientée exceptions.
   * Le verdict donne l'urgence ; la situation des Cibles et les repères
   * contextuels enrichissent ensuite naturellement la lecture. Rien ne
   * s'affiche en vert sans preuve récente. */

  import Topbar from '$lib/components/Topbar.svelte';
  import Spark from '$lib/components/Spark.svelte';
  import Bars from '$lib/components/Bars.svelte';
  import Uptime from '$lib/components/Uptime.svelte';
  import Odometer from '$lib/components/Odometer.svelte';
  import IndicatorOverview from '$lib/components/IndicatorOverview.svelte';
  import FleetOverview from '$lib/components/FleetOverview.svelte';
  import { coverageWindow } from '$lib/overview';
  import { session } from '$lib/session.svelte';
  import {
    calendarDay,
    duration,
    inWindow,
    lastObserved,
    leadIncident,
    ratio,
    since,
    stateLabel,
    stateTones,
    today
  } from '$lib/format';
  import { i18n, plural, t } from '$lib/i18n.svelte';

  let now = $state(new Date());

  $effect(() => {
    const timer = setInterval(() => (now = new Date()), 30_000);
    return () => clearInterval(timer);
  });

  /* Les Cibles à surveiller sont celles qui ne sont pas Opérationnelles. */
  const watched = $derived.by(() =>
    session.targets
      .map((target) => ({
        target,
        state: session.targetState(target),
        lead: leadIncident(session.incidentsFor(target.id)),
        measure: inWindow(session.measuresFor(target.id), '24h'),
        trend: session.measuresFor(target.id)?.latency_trend ?? [],
        lastObservedAt: lastObserved(target, session.measuresFor(target.id))
      }))
      .filter((row) => row.state !== 'ok')
      .sort((a, b) => a.target.name.localeCompare(b.target.name, i18n.locale))
  );

  const healthy = $derived(session.targets.length - watched.length);

  const critical = $derived(
    session.actionable.filter((incident) => incident.effective_severity === 'critical' || incident.effective_severity === 'major')
  );

  /* Couverture : part des Observations attendues qui ont réellement conclu,
   * toutes Cibles confondues. C'est elle qui révèle une défaillance de la
   * supervision elle-même, là où la Disponibilité resterait flatteuse. */
  const coverage = $derived.by(() => {
    const measured = Object.values(session.measures).map((entry) => inWindow(entry, '24h'));
    const expected = measured.reduce((total, measure) => total + measure.expected_observations, 0);
    if (expected === 0) return null;
    const conclusive = measured.reduce((total, measure) => total + measure.conclusive_observations, 0);
    return Math.min(1, conclusive / expected);
  });

  const freshest = $derived.by(() => {
    const stamps = session.targets
      .map((target) => lastObserved(target, session.measuresFor(target.id)))
      .filter((value): value is string => Boolean(value))
      .map((value) => new Date(value).getTime());
    return stamps.length > 0 ? new Date(Math.max(...stamps)) : null;
  });

  /* Les Incidents ouverts jour par jour, sous le compte du moment. La série
   * vient du serveur : c'est lui qui date les jours, et deux écrans ouverts
   * racontent donc le même passé. */
  const openedDays = $derived(session.incidentDays.map((day) => day.opened));

  const openedDayTooltips = $derived(
    session.incidentDays.map((day) =>
      plural('overview.fig.openedOnDay', day.opened, { date: calendarDay(day.day) })
    )
  );

  const openedTotal = $derived(
    session.incidentDays.length === 0
      ? null
      : session.incidentDays.reduce((total, day) => total + day.opened, 0)
  );

  /* La Couverture heure par heure de toute l'instance, lue sur la Santé qui
   * la publie déjà. Le chiffre et les barres parlent alors des mêmes heures :
   * sans quoi l'un démentirait l'autre. */
  const coverageHours = $derived(
    coverageWindow(session.system?.hours ?? [], session.system?.checked_at)
  );

  /* Une heure « aveugle » n'a rien conclu de ce qu'elle attendait. C'est elle
   * qui explique une Couverture entamée, et elle seule mérite la teinte
   * d'avertissement : une heure à peine grattée reste une heure vue. */
  const blindHours = $derived(
    (session.system?.hours ?? []).filter(
      (hour) => hour.expected_observations > 0 && hour.conclusive_observations === 0
    ).length
  );

  /* Les emplacements du micro-graphe de la dernière preuve : une Cible qui
   * a déjà conclu vaut un emplacement. Rien n'y est dessiné — la fraîcheur est
   * un état, pas une histoire — mais la forme dit sur combien de Cibles le
   * chiffre se fonde. */
  const evidenceSlots = $derived(
    session.targets.filter((target) => lastObserved(target, session.measuresFor(target.id))).length
  );

  /* L'intervalle médian entre deux Observations attendues, toutes Sources
   * actives confondues. La médiane plutôt que la moyenne : une seule Source
   * lente à l'heure ne doit pas déplacer la cadence que lisent les autres. */
  const medianInterval = $derived.by(() => {
    const intervals = session.targets
      .flatMap((target) => target.sources)
      .filter((source) => source.enabled && source.kind !== 'heartbeat')
      .map((source) => source.interval_seconds)
      .sort((left, right) => left - right);
    if (intervals.length === 0) return null;
    const middle = Math.floor(intervals.length / 2);
    return intervals.length % 2 === 0
      ? (intervals[middle - 1] + intervals[middle]) / 2
      : intervals[middle];
  });

  const verdict = $derived.by(() => {
    if (session.actionable.length === 0) {
      return {
        tone: 'ok' as const,
        title:
          session.targets.length === 0
            ? t('overview.verdict.noTargets')
            : t('overview.verdict.allClear'),
        say:
          session.targets.length === 0
            ? t('overview.verdict.noTargetsSay')
            : t('overview.verdict.allClearSay')
      };
    }
    if (critical.length > 0) {
      return {
        tone: 'crit' as const,
        title:
          critical.length > 1
            ? t('overview.verdict.manyDown', { count: critical.length })
            : t('overview.verdict.oneDown', { target: critical[0].target_name }),
        say: t('overview.verdict.downSay')
      };
    }
    return {
      tone: 'warn' as const,
      title: plural('overview.verdict.ongoing', session.actionable.length),
      say: t('overview.verdict.ongoingSay')
    };
  });

  const componentLabels = $derived<Record<string, string>>({
    server: t('component.server'),
    worker: t('component.worker'),
    postgresql: 'PostgreSQL'
  });

  const componentNotes = $derived<Record<string, string>>({
    server: t('component.serverNote'),
    worker: t('component.workerNote'),
    postgresql: t('component.postgresqlNote')
  });

</script>

<svelte:head><title>{t('overview.title')} — {session.instanceLabel}</title></svelte:head>

<Topbar crumbs={[{ label: t('nav.overview') }]} />

<div class="page">
  <div class="intro">
    <h1>{t('overview.title')}</h1>
    <p>{t('overview.lead')} <span class="faint">{today(now)}</span></p>
  </div>

  <div class="banner {verdict.tone} verdict">
    <div class="verdict-copy">
      <strong class={verdict.tone}><i class="dot {verdict.tone}"></i>{verdict.title}</strong>
      <p>{verdict.say}</p>
      <a class="more" href="/incidents">{t('overview.allIncidents')} →</a>
    </div>

    <div class="verdict-cells">
      <div class="cell">
        <span class="cell-title">{t('overview.fig.open')}</span>
        <b class={session.actionable.length > 0 ? verdict.tone : ''}>
          <Odometer value={session.actionable.length} />
        </b>
        <span class="graph dim">
          {#if openedDays.length > 0}
            <Bars
              values={openedDays}
              tooltips={openedDayTooltips}
              label={t('overview.fig.dailyHistory')}
            />
          {:else}
            <Bars mode="rule" />
          {/if}
        </span>
        <small class="incident-summary">
          <span class={session.unacknowledged.length > 0 ? 'crit' : 'faint'}>
            <span class="metric-number">{session.unacknowledged.length}</span>
            {plural('overview.fig.unacknowledgedCount', session.unacknowledged.length)}
          </span>
          <span class="faint" aria-hidden="true">·</span>
          <span class="faint">
            {#if openedTotal === null}
              {t('overview.fig.openUnread')}
            {:else}
              <span class="metric-number">{openedTotal}</span>
              {plural('overview.fig.openedLabel', openedTotal)}
              <span class="metric-number">{openedDays.length}</span>
              {plural('overview.fig.daysLabel', openedDays.length)}
            {/if}
          </span>
        </small>
      </div>

      <div class="cell">
        <span class="cell-title">{t('overview.fig.coverage')}</span>
        <b class:dim={coverage === null}><Odometer value={ratio(coverage)} /></b>
        <span class="graph">
          {#if coverageHours.length > 0}
            <Uptime values={coverageHours} />
          {:else}
            <Bars mode="rule" />
          {/if}
        </span>
        <small class={blindHours > 0 ? "warn" : "faint"}>
          {coverage === null
            ? t('overview.fig.coverageUnread')
            : blindHours > 0
              ? plural('overview.fig.blindHours', blindHours)
              : t('overview.fig.everyHourCovered')}
        </small>
      </div>

      <div class="cell">
        <span class="cell-title">{t('overview.fig.lastEvidence')}</span>
        <b class:dim={freshest === null}>
          <Odometer value={freshest ? since(freshest, now) : t('common.none')} />
        </b>
        <span class="graph dim">
          {#if evidenceSlots > 0}
            <Bars mode="slots" slots={evidenceSlots} />
          {:else}
            <Bars mode="rule" />
          {/if}
        </span>
        <small class="faint">
          {medianInterval === null
            ? t('overview.fig.noCadence')
            : t('overview.fig.medianInterval', { duration: duration(medianInterval) })}
        </small>
      </div>
    </div>
  </div>

  <FleetOverview />

  <IndicatorOverview />

  <div class="split">
    <section>
      <div class="band">
        <h2>{t('overview.watched')}</h2>
        <a class="more" href="/cibles">{t('overview.seeAll', { count: session.targets.length })} →</a>
      </div>

      <div class="card cols-watch">
        {#each watched as row (row.target.id)}
          <a class="trow" href="/cibles/{row.target.id}">
            <span class="cell-name">
              <i class="dot {stateTones[row.state]}"></i>
              <span>
                <strong>{row.target.name}</strong>
                {#if row.target.description}<small>{row.target.description}</small>{/if}
              </span>
            </span>

            <span class="{stateTones[row.state]} state">
              {stateLabel(row.state)}
              <small class="faint"><Odometer value={row.lastObservedAt
                ? t('overview.ago', { duration: since(row.lastObservedAt, now) })
                : t('overview.noEvidence')} /></small>
            </span>

            <span class="hide-sm trend {stateTones[row.state]}"><Spark values={row.trend} /></span>

            <span class="num hide-sm" class:dim={row.measure.availability === null}>
              <Odometer value={ratio(row.measure.availability)} />
              <small class="faint" title={t('overview.fig.coverage')}><Odometer value={ratio(row.measure.coverage)} /></small>
            </span>

            <span class="caret" aria-hidden="true">›</span>
          </a>
        {:else}
          <div class="empty">
            <strong>{t('overview.watchedEmpty')}</strong>
            {plural('overview.watchedEmptyHint', session.targets.length)}
          </div>
        {/each}

        {#if watched.length > 0 && healthy > 0}
          <a class="fold" href="/cibles">
            <i class="dot ok"></i>
            {plural('overview.healthyFold', healthy)}
            <span class="caret" aria-hidden="true">›</span>
          </a>
        {/if}
      </div>
    </section>

    <section>
      <div class="band">
        <h2>{t('overview.health')}</h2>
        <a class="more" href="/sante">{t('overview.diagnostic')} →</a>
      </div>

      <div class="card">
        <div class="card-body health">
          {#if session.system}
            {#each session.system.components as component (component.name)}
              <div class="comp">
                <span class="key">{componentLabels[component.name]?.slice(0, 2).toUpperCase() ?? '··'}</span>
                <span class="comp-id">
                  <strong>{componentLabels[component.name] ?? component.name}</strong>
                  <small class="faint">{componentNotes[component.name] ?? ''}</small>
                </span>
                <span class={component.status === 'operational' ? 'ok' : component.status === 'stale' ? 'warn' : 'crit'}>
                  <i class="dot {component.status === 'operational' ? 'ok' : component.status === 'stale' ? 'warn' : 'crit'}"></i>
                  {t(`component.status.${component.status}`)}
                </span>
              </div>
            {/each}
          {:else}
            <p class="faint">{t('overview.healthUnread')}</p>
          {/if}

          {#if session.connectors.length > 0}
            <div class="comp connectors">
              <span class="comp-id"><strong>{t('nav.connectors')}</strong></span>
              <span class="faint num">
                {t('overview.connected', {
                  connected: session.connectors.filter(
                    (connector) => connector.status === 'connected'
                  ).length,
                  total: session.connectors.length
                })}
              </span>
            </div>
          {/if}
        </div>
      </div>
    </section>
  </div>
</div>

<style>
  .intro {
    margin-bottom: var(--s5);
  }

  .intro h1 {
    font-size: 1.375rem;
    margin-bottom: 0.375rem;
  }

  .intro p {
    color: var(--muted);
    font-size: 0.8125rem;
  }

  /* Le verdict et ses cellules chiffrées. Le verdict garde le lavis de son
     ton ; les cellules reviennent à la surface neutre, parce qu'un chiffre
     n'est pas un verdict — la Couverture ne devient pas rassurante du seul
     fait que rien ne brûle. */
  .banner.verdict {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 2fr);
    align-items: stretch;
    gap: 0;
    margin-bottom: var(--s5);
    padding: 0;
    overflow: hidden;
  }

  .verdict-copy {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: var(--s2);
    padding: var(--s4) var(--s5);
    min-width: 0;
  }

  .verdict-copy strong {
    display: flex;
    align-items: center;
    gap: var(--s3);
    font-size: 0.9375rem;
    font-weight: 600;
  }

  .verdict-copy p {
    color: var(--muted);
    font-size: 0.8125rem;
  }

  .verdict-copy .more {
    margin: var(--s1) 0 0;
  }

  /* Les cellules reposent sur la surface : le lavis du verdict s'arrête au
     filet, et les micro-graphes gardent leurs propres teintes. */
  .verdict-cells {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    background: var(--surface);
    border-left: 1px solid var(--line-strong);
    /* Le ton du verdict colore son propre bloc, pas les chiffres : une
       Couverture entamée ne devient pas verte parce que rien ne brûle. Chaque
       cellule reprend donc la teinte quand elle a elle-même quelque chose à
       signaler. */
    color: var(--ink);
  }

  .cell {
    display: grid;
    align-content: start;
    gap: var(--s3);
    padding: var(--s4);
    border-left: 1px solid var(--line);
    min-width: 0;
  }

  .cell:first-child {
    border-left: 0;
  }

  .cell-title {
    color: var(--muted);
    font-size: 0.75rem;
  }

  .cell b {
    font-family: var(--font-num);
    font-variant-numeric: tabular-nums;
    font-size: 1.375rem;
    font-weight: 600;
    line-height: 1;
  }

  .cell small {
    font-size: 0.6875rem;
  }

  .incident-summary {
    display: flex;
    flex-wrap: wrap;
    gap: var(--s2);
  }

  .metric-number {
    font-family: var(--font-num);
    font-variant-numeric: tabular-nums;
  }

  .graph {
    display: block;
    color: var(--dim);
  }

  /* Une bande ne porte que son espace bas. L'espace haut appartient au bloc
     qui la contient — sans quoi `:first-of-type`, qui compte les div et non
     les bandes, l'annulait dans chaque colonne du bloc en deux parties et
     l'en-tête venait coller la dalle du dessus. */
  .band {
    display: flex;
    align-items: center;
    gap: 0.5625rem;
    margin: 0 0 var(--s4);
  }

  .page > .band {
    margin-top: var(--s6);
  }

  .band h2 {
    font-size: 0.9375rem;
    font-weight: 600;
  }

  .more {
    margin-left: auto;
    color: var(--muted);
    font-size: 0.75rem;
  }

  .more:hover {
    color: var(--accent);
  }

  .cols-watch {
    --cols: minmax(0, 1fr) 9.375rem 5.125rem 5.75rem 1.25rem;
  }

  .state {
    font-size: 0.75rem;
  }

  .state small,
  .num small {
    display: block;
    margin-top: 2px;
    font-size: 0.6875rem;
  }

  .trend { color: var(--ok) }
  .trend.crit { color: var(--crit) }
  .trend.warn { color: var(--warn) }
  .trend.info { color: var(--info) }
  .trend.idle { color: var(--dim) }

  .split {
    display: grid;
    grid-template-columns: minmax(0, 1.75fr) minmax(0, 1fr);
    gap: var(--s5);
    align-items: start;
    margin-top: var(--s6);
  }

  .fold {
    display: flex;
    align-items: center;
    gap: 0.5625rem;
    padding: 0.6875rem 1rem;
    border-top: 1px solid var(--line);
    color: var(--muted);
    font-size: 0.75rem;
  }

  .fold:hover {
    background: var(--bg);
    color: var(--ink);
  }

  .fold .caret {
    margin-left: auto;
  }

  .health {
    display: grid;
    gap: 0;
  }

  .comp {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    padding: 0.5625rem 0;
    border-bottom: 1px solid var(--line-row);
    font-size: 0.75rem;
  }

  .comp:last-child {
    border-bottom: 0;
  }

  .comp > span:last-child {
    margin-left: auto;
    display: inline-flex;
    align-items: center;
    gap: 0.3125rem;
    white-space: nowrap;
  }

  .key {
    width: 1.625rem;
    height: 1.625rem;
    flex: none;
    display: grid;
    place-items: center;
    border-radius: var(--r-s);
    background: var(--surface-2);
    color: var(--muted);
    font-size: 0.625rem;
    font-weight: 600;
  }

  .comp-id strong {
    display: block;
    font-size: 0.75rem;
    font-weight: 600;
  }

  .comp-id small {
    display: block;
    font-size: 0.6875rem;
  }

  .connectors {
    padding-top: var(--s4);
    border-top: 1px solid var(--line);
    border-bottom: 0;
  }

  @media (max-width: 68rem) {
    .split {
      grid-template-columns: minmax(0, 1fr);
    }

    /* Sous cette largeur, les cellules passent sous le verdict : le filet
       vertical qui les en séparait redevient horizontal. */
    .banner.verdict {
      grid-template-columns: minmax(0, 1fr);
    }

    .verdict-cells {
      border-left: 0;
      border-top: 1px solid var(--line-strong);
    }
  }

  /* Trois cellules ne tiennent plus en ligne sur un téléphone : elles
     s'empilent, et leurs filets suivent. */
  @media (max-width: 48rem) {
    .verdict-cells {
      grid-template-columns: minmax(0, 1fr);
    }

    .cell {
      border-left: 0;
    }

    .cell:not(:first-child) {
      border-top: 1px solid var(--line);
    }
  }
</style>

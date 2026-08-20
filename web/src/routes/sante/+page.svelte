<script lang="ts">
  /* Écran 4f — Santé CairnOps.
   * « Si cette page ment, tout le reste ment aussi. » Elle ne montre donc que
   * ce que l'instance rapporte, et dit explicitement ce qu'elle ignore. */

  import Topbar from '$lib/components/Topbar.svelte';
  import Icon, { type IconName } from '$lib/components/Icon.svelte';
  import BrandMark, { type BrandName } from '$lib/components/BrandMark.svelte';
  import Bars from '$lib/components/Bars.svelte';
  import Odometer from '$lib/components/Odometer.svelte';
  import { session } from '$lib/session.svelte';
  import { latency, percent, since, stamp } from '$lib/format';
  import { plural, t } from '$lib/i18n.svelte';

  let now = $state(new Date());

  $effect(() => {
    const timer = setInterval(() => (now = new Date()), 15_000);
    return () => clearInterval(timer);
  });

  const labels = $derived<Record<string, { name: string; note: string }>>({
    server: { name: t('component.server'), note: t('health.serverNote') },
    worker: { name: t('component.worker'), note: t('health.workerNote') },
    postgresql: { name: 'PostgreSQL', note: t('component.postgresqlNote') }
  });

  /* Chaque composant porte le geste qu'il fait : le serveur répond, le worker
   * tourne en boucle, PostgreSQL empile. Un composant que l'instance nommerait
   * sans que l'écran le connaisse garde le trait générique. */
  const componentIcons: Record<string, IconName> = {
    server: 'server',
    worker: 'worker',
    postgresql: 'database'
  };

  /* Les Connecteurs reprennent la marque montrée sur leur propre écran : un
   * produit tiers se reconnaît à son logo, pas à ses deux initiales. */
  const kindBrands: Partial<Record<string, BrandName>> = {
    zabbix: 'zabbix',
    uptime_kuma: 'uptime_kuma'
  };

  const statusLabels = $derived<Record<string, { label: string; tone: string }>>({
    operational: { label: t('component.status.operational'), tone: 'ok' },
    stale: { label: t('component.status.stale'), tone: 'warn' },
    unavailable: { label: t('component.status.unavailable'), tone: 'crit' }
  });

  const activeSources = $derived(
    session.targets.reduce((total, target) => total + target.sources.filter((source) => source.enabled).length, 0)
  );

  /* La Couverture du bandeau se lit sur la même série horaire que son
   * micro-graphe : le chiffre et les barres parlent des mêmes heures, sinon
   * l'un dément l'autre. */
  const hours = $derived(session.system?.hours ?? []);

  const executed = $derived(
    hours.reduce(
      (total, hour) => ({
        expected: total.expected + hour.expected_observations,
        conclusive: total.conclusive + hour.conclusive_observations,
        healthy: total.healthy + hour.healthy_observations
      }),
      { expected: 0, conclusive: 0, healthy: 0 }
    )
  );

  const coverage = $derived(
    executed.expected === 0 ? null : Math.min(1, executed.conclusive / executed.expected)
  );

  const coverageTone = $derived(
    coverage === null ? 'dim' : coverage >= 0.99 ? 'ok' : coverage >= 0.9 ? 'warn' : 'crit'
  );

  /* Une heure sans attente ne pèse pas sur la barre : elle n'a rien promis. */
  const coverageBars = $derived(
    hours.map((hour) =>
      hour.expected_observations === 0
        ? 1
        : Math.min(1, hour.conclusive_observations / hour.expected_observations)
    )
  );

  const database = $derived(session.system?.database ?? null);

  /* Une Source est « en retard » quand sa dernière Observation dépasse deux
   * fois son intervalle : le worker n'a pas tenu la cadence annoncée. */
  const overdue = $derived(
    session.targets
      .flatMap((target) => target.sources)
      .filter((source) => {
        if (!source.enabled || source.kind === 'heartbeat') return false;
        if (!source.last_observed_at) return true;
        const age = (now.getTime() - new Date(source.last_observed_at).getTime()) / 1000;
        return age > source.interval_seconds * 2;
      }).length
  );

  const globalTone = $derived(
    session.system?.status === 'operational' ? 'ok' : session.system ? 'warn' : 'crit'
  );
</script>

<svelte:head><title>{t('health.title')} — {session.instanceLabel}</title></svelte:head>

<Topbar crumbs={[{ label: t('nav.health') }]} />

<div class="page">
  <div class="page-head">
    <div>
      <h1>
        {t('health.title')}
        <span class="pill {globalTone}">
          {session.system?.status === 'operational'
            ? t('health.stable')
            : session.system
              ? t('health.degraded')
              : t('state.unknown')}
        </span>
      </h1>
      <p>{t('health.lead')}</p>
    </div>
  </div>

  <div class="card">
    <div class="card-body cells">
      <!-- Chaque cellule tient son chiffre, son passé et sa légende. Le
           micro-graphe couvre exactement la fenêtre annoncée par le titre. -->
      <div class="cell">
        <span class="cell-title">{t('overview.fig.coverage')}</span>
        <b class:dim={coverage === null}><Odometer value={coverage === null ? t('common.none') : percent(coverage)} /></b>
        <span class="graph {coverageTone}">
          {#if coverageBars.length > 0}
            <Bars values={coverageBars} />
          {:else}
            <Bars mode="rule" />
          {/if}
        </span>
        <small class={coverage === null ? 'faint' : coverageTone}>
          {executed.expected === 0
            ? t('health.noExecutions')
            : t('health.executions', {
                executed: executed.conclusive.toLocaleString('fr-FR'),
                healthy: executed.healthy.toLocaleString('fr-FR')
              })}
        </small>
      </div>

      <div class="cell">
        <span class="cell-title">{t('health.activeChecks')}</span>
        <b><Odometer value={activeSources} /></b>
        <!-- Un effectif n'a pas d'histoire horaire : les emplacements restent
             vides plutôt que de tracer une ligne inventée. -->
        <span class="graph"><Bars mode="slots" /></span>
        <small class="faint">
          {t('health.overSupervised', { count: session.targets.length })}
        </small>
      </div>

      <div class="cell">
        <span class="cell-title">{t('health.overdue')}</span>
        <b class={overdue > 0 ? 'warn' : ''}><Odometer value={overdue} /></b>
        <span class="graph {overdue > 0 ? 'warn' : 'faint'}"><Bars mode="rule" /></span>
        <small class="faint">
          {overdue > 0 ? t('health.overdueHint') : t('health.onCadence')}
        </small>
      </div>

      <div class="cell">
        <span class="cell-title">{t('health.databaseLatency')}</span>
        <b class:dim={database === null}>
          <Odometer value={database === null ? t('common.none') : latency(Math.round(database.latency_milliseconds))} />
        </b>
        <span class="graph muted">
          {#if database && database.samples.length > 1}
            <Bars values={database.samples} slots={database.samples.length} />
          {:else}
            <Bars mode="rule" />
          {/if}
        </span>
        <small class="faint">
          {database === null
            ? t('health.databaseUnread')
            : t('health.databaseHint', { maximum: Math.round(database.maximum_latency_milliseconds) })}
        </small>
      </div>
    </div>
  </div>

  <h2 class="band">{t('health.components')}</h2>

  <div class="card">
    {#if session.system}
      {#each session.system.components as component (component.name)}
        {@const status = statusLabels[component.status] ?? { label: component.status, tone: 'idle' }}
        <div class="comp">
          <span class="key"><Icon name={componentIcons[component.name] ?? 'health'} size={16} /></span>
          <span class="comp-id">
            <strong>{labels[component.name]?.name ?? component.name}</strong>
            <small class="faint">{labels[component.name]?.note ?? ''}</small>
          </span>
          <span class="faint num hide-sm">
            {plural('health.instances', component.instances)}
            {#if component.last_seen_at}
              · {t('health.seenAgo', { duration: since(component.last_seen_at, now) })}
            {/if}
          </span>
          <span class={status.tone}><i class="dot {status.tone}"></i>{status.label}</span>
        </div>
      {/each}
    {:else}
      <div class="empty">
        <strong>{t('overview.healthUnread')}</strong>
        {t('health.apiSilent')} <code>/api/v1/system/health</code>.
      </div>
    {/if}
  </div>

  <h2 class="band">{t('nav.connectors')}</h2>

  <div class="card">
    {#each session.connectors as connector (connector.id)}
      {@const tone = connector.status === 'connected' ? 'ok' : connector.status === 'degraded' ? 'warn' : 'idle'}
      <div class="comp">
        {#if kindBrands[connector.kind]}
          <BrandMark name={kindBrands[connector.kind]!} size={28} />
        {:else}
          <span class="key"><Icon name="webhook" size={16} /></span>
        {/if}
        <span class="comp-id">
          <strong>{connector.name}</strong>
          <small class="faint">{connector.endpoint}</small>
        </span>
        <span class="faint num hide-sm">
          {#if connector.last_checked_at}
            {t('health.signalAgo', { duration: since(connector.last_checked_at, now) })}
          {:else}
            {t('health.neverChecked')}
          {/if}
        </span>
        <span class={tone}>
          <i class="dot {tone}"></i>
          {t(`connector.status.${connector.status}`)}
        </span>
      </div>
    {:else}
      <div class="empty">
        <strong>{t('health.noConnectors')}</strong>
        {t('health.noConnectorsHint')}
      </div>
    {/each}
  </div>

  <h2 class="band">{t('health.instance')}</h2>

  <div class="card">
    <div class="comp">
      <span class="comp-id"><strong>{t('health.version')}</strong></span>
      <span class="num">v{session.version}</span>
    </div>
    <div class="comp">
      <span class="comp-id"><strong>{t('health.lastRead')}</strong></span>
      <span class="num">{session.system ? stamp(session.system.checked_at) : t('common.none')}</span>
    </div>
    <div class="comp">
      <span class="comp-id"><strong>{t('health.session')}</strong></span>
      <span class="muted">
        {session.user?.display_name ?? t('common.none')}
        {#if session.user}· {t(`role.${session.user.role}`)}{/if}
      </span>
    </div>
  </div>
</div>

<style>
  /* Bandeau 6b — cellules chiffrées à micro-graphes. Une colonne par cellule,
     séparées d'un filet plutôt que d'une carte chacune : c'est une lecture,
     pas quatre. */
  .cells {
    display: grid;
    grid-template-columns: repeat(4, minmax(0, 1fr));
    gap: 0;
    padding: 0;
  }

  .cell {
    display: grid;
    gap: 0.5rem;
    padding: var(--s4) 1rem;
    border-left: 1px solid var(--line);
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
    font-size: 1.5rem;
    font-weight: 600;
    line-height: 1;
  }

  .cell small {
    font-size: 0.6875rem;
  }

  .graph {
    display: block;
    color: var(--dim);
  }

  .graph.ok { color: var(--ok) }
  .graph.warn { color: var(--warn) }
  .graph.crit { color: var(--crit) }
  .graph.muted { color: var(--muted) }
  .graph.faint { color: var(--faint) }

  @media (max-width: 60rem) {
    .cells {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    /* En deux colonnes, le filet vertical de la troisième cellule retomberait
       en tête de rangée : il redevient un filet horizontal. */
    .cell:nth-child(odd) {
      border-left: 0;
    }

    .cell:nth-child(n + 3) {
      border-top: 1px solid var(--line);
    }
  }

  @media (max-width: 34rem) {
    .cells {
      grid-template-columns: minmax(0, 1fr);
    }

    .cell {
      border-left: 0;
    }

    .cell:not(:first-child) {
      border-top: 1px solid var(--line);
    }
  }

  .page-head h1 {
    display: flex;
    align-items: center;
    gap: 0.625rem;
  }

  .band {
    margin: var(--s6) 0 var(--s4);
    font-size: 0.9375rem;
    font-weight: 600;
  }

  .comp {
    display: flex;
    align-items: center;
    gap: 0.625rem;
    padding: 0.6875rem 1rem;
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
    width: 1.75rem;
    height: 1.75rem;
    flex: none;
    display: grid;
    place-items: center;
    border-radius: var(--r-s);
    background: var(--surface-2);
    color: var(--muted);
    font-size: 0.625rem;
    font-weight: 600;
  }

  .comp-id {
    flex: 1;
    min-width: 0;
  }

  .comp-id strong {
    display: block;
    font-size: 0.8125rem;
    font-weight: 600;
  }

  .comp-id small {
    display: block;
    font-family: var(--font-num);
    font-size: 0.6875rem;
  }

  code {
    font-family: var(--font-num);
    font-size: 0.6875rem;
  }
</style>

<script lang="ts">
  // Atelier jetable : trois compositions (?variant=A/B/C), état en mémoire.
  // Intent : comprendre les exceptions, puis passer du verdict aux preuves.
  // Hiérarchie : état global > incidents > contexte > inventaire sain.
  // Palette : graphite/pierre, cuivre d’action, couleurs de santé réservées.
  // Depth/surfaces : traits fins et élévation par surface, aucun décor.
  // Typographie : système 28/18/14/13/12, nombres tabulaires. Grille : 4 px.
  import { onMount } from 'svelte';
  import Icon, { type IconName } from '../src/lib/components/Icon.svelte';
  import Chart from './Chart.svelte';
  import ThemePicker from './ThemePicker.svelte';
  import { targets, incidents, type Target } from './data';

  const params = new URLSearchParams(location.search);
  const variants = ['A','B','C'];
  let variant = $state(variants.includes(params.get('variant') ?? '') ? params.get('variant')! : 'A');
  let theme = $state<'light'|'dark'>('dark');
  let section = $state('overview');
  let query = $state('');
  let filter = $state('all');
  let range = $state('24h');
  let page = $state(0);
  let acknowledged = $state<number[]>([]);
  let selected = $state<Target>(targets[0]);
  let toast = $state('');
  let dialog = $state<HTMLDialogElement>(null!);
  let searchInput: HTMLInputElement;
  let toastTimer: ReturnType<typeof setTimeout>;
  let trigger: HTMLElement | null = null;
  const perPage = 5;
  const nav: {id:string;label:string;icon:IconName;count?:string}[] = [
    {id:'overview',label:'Vue d’ensemble',icon:'overview'},
    {id:'targets',label:'Cibles',icon:'targets',count:'48'},
    {id:'incidents',label:'Incidents',icon:'incidents',count:'2'},
    {id:'maintenance',label:'Maintenance',icon:'maintenance'},
    {id:'connectors',label:'Connecteurs',icon:'connectors',count:'3'},
  ];
  const medianLatency = [...targets].flatMap(t=>t.latency===null?[]:[t.latency]).sort((a,b)=>a-b)[23];
  const filtered = $derived(targets.filter(t=>(filter==='all'||t.tone!=='ok') && `${t.name} ${t.source}`.toLocaleLowerCase('fr').includes(query.toLocaleLowerCase('fr'))));
  const visible = $derived(filtered.slice(page*perPage,(page+1)*perPage));
  const pending = $derived(2-acknowledged.length);
  const incident = $derived(incidents.find(i=>i.id===selected.id));
  const variantNames: Record<string,string> = {A:'Vue d’ensemble',B:'Priorité aux incidents',C:'Analyse en grand'};
  const themeChange = (value:'light'|'dark') => { theme=value; document.documentElement.dataset.theme=value; };
  function chooseVariant(value:string) {
    variant=value;
    const url=new URL(location.href); url.searchParams.set('variant',value); history.replaceState({},'',url);
  }
  function moveVariant(delta:number) { chooseVariant(variants[(variants.indexOf(variant)+delta+3)%3]); }
  function navigate(id:string) { section=id; query='';filter='all';page=0; }
  function detail(target:Target,event?:MouseEvent) { selected=target;trigger=event?.currentTarget instanceof HTMLElement?event.currentTarget:null;dialog.showModal(); }
  function notify(message:string) { toast=message;clearTimeout(toastTimer);toastTimer=setTimeout(()=>toast='',3600); }
  function acknowledge(id:number) { if(!acknowledged.includes(id))acknowledged=[...acknowledged,id];notify('Incident acquitté dans la démonstration. La supervision continue.'); }
  function key(event:KeyboardEvent) {
    const target=event.target instanceof HTMLElement?event.target:null;
    if ((event.metaKey||event.ctrlKey)&&event.key==='k') { event.preventDefault();navigate('targets');setTimeout(()=>searchInput?.focus(),0);return; }
    if (dialog?.open || target?.closest('input,textarea,select,[contenteditable],button,[role="slider"],summary'))return;
    if(event.key==='ArrowRight')moveVariant(1);
    if(event.key==='ArrowLeft')moveVariant(-1);
  }
  onMount(()=>()=>clearTimeout(toastTimer));
</script>

<svelte:window onkeydown={key}/>

{#snippet logo(size=28)}
  <svg width={size} height={size} viewBox="0 0 32 32" fill="none" aria-hidden="true" class="p-logo"><path d="M12 4h8a3 3 0 0 1 3 3v1H9V7a3 3 0 0 1 3-3Z" fill="currentColor"/><path d="M8 12h16a3 3 0 0 1 3 3v2H5v-2a3 3 0 0 1 3-3Z" fill="currentColor" opacity=".8"/><path d="M5 21h22a3 3 0 0 1 3 3v3H2v-3a3 3 0 0 1 3-3Z" fill="currentColor" opacity=".55"/></svg>
{/snippet}

{#snippet incidentCard(item:typeof incidents[number])}
  <article class="p-incident-card">
    <div class="p-incident-top"><span class={`p-badge ${item.tone}`}><span class="p-dot"></span>{item.severity}</span><span class="p-small">{item.duration}</span></div>
    <button class="p-incident-title" onclick={event=>detail(targets[item.id],event)}>{item.target}<span>↗</span></button>
    <p>{item.title}</p>
    <div class="p-proof-label"><Icon name="signal" size={13}/>{item.source}<span>·</span>{item.assurance}</div>
    <div class="p-incident-actions"><span class="p-small">{item.since}</span>{#if acknowledged.includes(item.id)}<span class="p-ack"><Icon name="health" size={13}/>Acquitté</span>{:else}<button class="p-btn p-btn-small" onclick={()=>acknowledge(item.id)}>Acquitter<Icon name="plus" size={12}/></button>{/if}</div>
  </article>
{/snippet}

{#snippet metrics()}
  <section class="p-metrics" aria-label="Synthèse de la supervision">
    <article class="p-metric"><div class="p-metric-label">Cibles opérationnelles<Icon name="targets" size={16}/></div><div class="p-metric-number">46<span>/ 48</span></div><div class="p-fleet-track" aria-label="46 opérationnelles, 1 dégradée, 1 indisponible">{#each [...targets].reverse() as target}<i class={target.tone}></i>{/each}</div><div class="p-metric-note"><span class="p-dot warn"></span>2 cibles à surveiller</div></article>
    <article class="p-metric"><div class="p-metric-label">Disponibilité · 24 h<Icon name="health" size={16}/></div><div class="p-metric-number">99,94<span>%</span></div><div class="p-metric-rule"><span></span><i></i></div><div class="p-metric-note">Couverture <b>100 %</b><span class="p-quiet">· 48 cibles</span></div></article>
    <article class="p-metric"><div class="p-metric-label">Latence médiane<Icon name="signal" size={16}/></div><div class="p-metric-number">{medianLatency}<span>ms</span><div class="p-metric-mini"><Chart mini seed={3} interactive={false} label="Tendance de latence, données de démonstration"/></div></div><div class="p-metric-note">Dernières mesures<span class="p-quiet">· 47 cibles</span></div></article>
    <article class="p-metric"><div class="p-metric-label">Sources configurées<Icon name="connectors" size={16}/></div><div class="p-metric-number">64<span>sources</span></div><div class="p-source-mini"><span>CairnOps <b>22</b></span><span>Kuma <b>18</b></span><span>Zabbix <b>24</b></span></div></article>
  </section>
{/snippet}

{#snippet responseChart()}
  <section class="p-panel p-response">
    <header class="p-panel-head"><div><h2>Temps de réponse</h2><p>API publique <span>·</span> Uptime Kuma</p></div><div class="p-segments" role="group" aria-label="Période du graphique">{#each ['1h','24h','7j'] as r}<button class:active={range===r} aria-pressed={range===r} onclick={()=>range=r}>{r==='7j'?'7 jours':r==='24h'?'24 heures':'1 heure'}</button>{/each}</div></header>
    <div class="p-chart-heading"><strong>184 <span>ms</span></strong><span class="p-context-label"><span class="p-line-sample"></span>Indicateur contextuel</span></div>
    {#key range}<div class="p-chart-enter"><Chart {range}/></div>{/key}
    <footer class="p-chart-footer"><span><span class="p-dot warn"></span>Temps de réponse élevé depuis 14:18</span><button onclick={event=>detail(targets[0],event)}>Voir les preuves <span>↗</span></button></footer>
  </section>
{/snippet}

{#snippet incidentPanel()}
  <section class="p-panel p-incidents"><header class="p-panel-head"><div><h2>Incidents en cours <span class="p-count">2</span></h2><p>{pending>0?`${pending} à acquitter`:'Tous pris en charge'} <span>·</span> supervision active</p></div><Icon name="incidents" size={18}/></header><div class="p-incident-items">{#each incidents as item}{@render incidentCard(item)}{/each}</div></section>
{/snippet}

{#snippet targetTable()}
  <section class="p-panel p-targets">
    <header class="p-panel-head"><div><h2>Cibles</h2><p>Le même état, toutes sources confondues.</p></div><div class="p-table-tools"><div class="p-segments" role="group" aria-label="Filtrer les cibles"><button class:active={filter==='all'} aria-pressed={filter==='all'} onclick={()=>{filter='all';page=0;}}>Toutes <span>48</span></button><button class:active={filter==='watch'} aria-pressed={filter==='watch'} onclick={()=>{filter='watch';page=0;}}>À surveiller <span>2</span></button></div><label class="p-table-search"><Icon name="search" size={15}/><input bind:this={searchInput} bind:value={query} oninput={()=>page=0} placeholder="Rechercher une cible…" aria-label="Rechercher une cible"/><kbd>⌘ K</kbd></label></div></header>
    <div class="p-table-wrap"><table class="p-table"><thead><tr><th>Cible</th><th>État de santé</th><th>Latence</th><th>Disponibilité <span>24 h</span></th><th>Source</th><th><span class="p-sr">Détail</span></th></tr></thead><tbody>
      {#each visible as target}<tr><td><button class="p-target-name" onclick={event=>detail(target,event)}><span class="p-target-icon"><Icon name={target.category==='Stockage'?'database':target.category==='Tâche planifiée'?'worker':target.category==='Réseau'?'signal':'server'} size={17}/></span><span><strong>{target.name}</strong><small>{target.category}</small></span></button></td><td><span class={`p-state ${target.tone}`}><span class="p-dot"></span>{target.state}</span></td><td class="p-numeric">{target.latency??'—'}{#if target.latency!==null}<span>ms</span>{/if}</td><td><div class="p-availability"><span class="p-numeric">{target.availability}<span>%</span></span><div class="p-uptime-bars" aria-hidden="true">{#each Array(28) as _,i}<i class:interrupted={target.id===1&&i===26}></i>{/each}</div></div><small class="p-coverage">Couverture {target.coverage} %</small></td><td><span class="p-source-name"><span class="p-source-symbol">{target.source==='CairnOps'?'C':target.source==='Zabbix'?'Z':'U'}</span>{target.source}{#if target.sourceCount>1}<span class="p-quiet">+1</span>{/if}</span></td><td><button class="p-icon-btn" aria-label={`Détails de ${target.name}`} onclick={event=>detail(target,event)}>↗</button></td></tr>{/each}
      {#if visible.length===0}<tr><td colspan="6"><div class="p-empty"><Icon name="search" size={28}/><strong>Aucune cible trouvée</strong><p>Essayez un autre nom ou retirez le filtre.</p><button class="p-btn" onclick={()=>{query='';filter='all';page=0;}}>Afficher les cibles</button></div></td></tr>{/if}
    </tbody></table></div>
    <footer class="p-table-footer"><span>{filtered.length?`${page*perPage+1}–${Math.min((page+1)*perPage,filtered.length)} sur ${filtered.length} cibles`:'0 cible'}</span><span class="p-small">Observations du scénario · 14:32</span><div><button class="p-icon-btn" aria-label="Page précédente" disabled={page===0} onclick={()=>page--}>←</button><button class="p-icon-btn" aria-label="Page suivante" disabled={(page+1)*perPage>=filtered.length} onclick={()=>page++}>→</button></div></footer>
  </section>
{/snippet}

  <div class="p-app" data-variant={variant}>
    <a class="p-skip" href="#content">Aller au contenu</a>
    <aside class="p-sidebar"><a class="p-brand" href="?variant=A" aria-label="CairnOps, vue d’ensemble">{@render logo()}<span>CairnOps</span></a><div class="p-space"><span class="p-space-icon">H</span><span><strong>Homeblack</strong><small>Espace opérationnel</small></span><span class="p-chevron">⌄</span></div><p class="p-nav-label">Supervision</p><nav aria-label="Navigation principale">{#each nav as item}<button class:current={section===item.id} aria-current={section===item.id?'page':undefined} onclick={()=>navigate(item.id)}><Icon name={item.icon} size={17}/><span>{item.label}</span>{#if item.count}<b class:hot={item.id==='incidents'}>{item.count}</b>{/if}</button>{/each}</nav><div class="p-sidebar-bottom"><div class="p-instance"><span class="p-dot ok"></span><span>Instance opérationnelle</span></div><div class="p-person"><span class="p-avatar">GN</span><span><strong>Grégory</strong><small>Administrateur · démo</small></span><Icon name="settings" size={16}/></div></div></aside>
    <div class="p-workspace"><header class="p-topbar"><div class="p-breadcrumb"><span>Homeblack</span><span>/</span><strong>{nav.find(i=>i.id===section)?.label}</strong></div><div class="p-topbar-actions"><span class="p-demo-label">Données de démonstration</span><ThemePicker ontheme={themeChange}/><button class="p-icon-btn p-notifications" aria-label="Voir les incidents" onclick={()=>navigate('incidents')}><Icon name="bell" size={18}/>{#if pending>0}<i></i>{/if}</button></div></header>
    <main id="content" class="p-content" tabindex="-1">
      <header class="p-page-head"><div><div class="p-eyebrow">{section==='overview'?'Votre espace opérationnel':'Homeblack'}</div><h1>{nav.find(i=>i.id===section)?.label}</h1><p>{section==='overview'?'48 cibles. 64 sources. Une situation partagée.':section==='targets'?'Chaque cible, ses signaux, ses preuves.':section==='incidents'?'Comprendre, prendre en charge, suivre le rétablissement.':section==='connectors'?'Vos outils de supervision, réunis au même endroit.':'Anticiper les interventions, garder le contexte.'}</p></div><span class="p-date"><Icon name="changelog" size={15}/>5 septembre 2026</span></header>
      {#if section==='overview'}
        <div class="p-verdict"><span class="p-verdict-icon"><Icon name="incidents" size={19}/></span><div><strong>1 cible indisponible, 1 cible dégradée</strong><span>{pending>0?`${pending} incident${pending>1?'s':''} en attente de prise en charge.`:'Les deux incidents sont pris en charge.'} Dernières observations à 14:32.</span></div><button onclick={()=>navigate('incidents')}>Examiner <span>→</span></button></div>
        <div class="p-composition">
          <div class="p-metric-slot">{@render metrics()}</div>
          <div class="p-analysis-slot">{@render responseChart()}</div>
          <div class="p-incident-slot">{@render incidentPanel()}</div>
        </div>
        {@render targetTable()}
      {:else if section==='targets'}{@render targetTable()}
      {:else if section==='incidents'}<div class="p-incident-full">{@render incidentPanel()}{@render responseChart()}</div>
      {:else if section==='connectors'}
        <div class="p-connector-grid">{#each [{name:'CairnOps',icon:'signal' as const,description:'Contrôles natifs et heartbeats',count:22},{name:'Uptime Kuma',icon:'health' as const,description:'Disponibilité et temps de réponse',count:18},{name:'Zabbix',icon:'server' as const,description:'Hôtes, signaux et indicateurs',count:24}] as connector}<article class="p-panel p-connector"><div class="p-connector-mark"><Icon name={connector.icon} size={25}/></div><span class="p-badge ok"><span class="p-dot"></span>Connecté</span><h2>{connector.name}</h2><p>{connector.description}</p><div><strong>{connector.count}</strong><span>sources configurées</span></div><button class="p-btn" onclick={()=>{navigate('targets');query=connector.name;}}>Voir les cibles <span>→</span></button></article>{/each}</div>
      {:else if section==='maintenance'}
        <section class="p-panel p-maintenance"><span class="p-badge info"><Icon name="maintenance" size={14}/>Planifiée</span><h2>Mise à jour du stockage</h2><p>Dimanche 6 septembre · 02:00–03:00 · Stockage principal</p><div class="p-maintenance-explanation"><Icon name="bell" size={19}/><p>Les observations restent enregistrées. Pendant la fenêtre, les incidents de cette cible n’affectent ni l’état global, ni la disponibilité, ni les notifications.</p></div><button class="p-btn" onclick={event=>detail(targets[4],event)}>Voir la cible <span>→</span></button></section>
      {/if}
      <footer class="p-workspace-footer"><span>{@render logo(15)}CairnOps</span><span>Un verdict lisible. Des preuves accessibles.</span><span>Maquette interactive</span></footer>
    </main></div>
    <div class="p-prototype-bar" aria-label="Choisir une composition"><span class="p-prototype-label">Atelier</span><button aria-label="Composition précédente" onclick={()=>moveVariant(-1)}>←</button><span><b>{variant}</b> {variantNames[variant]}</span><button aria-label="Composition suivante" onclick={()=>moveVariant(1)}>→</button><span class="p-prototype-theme">{theme==='dark'?'Sombre':'Clair'}</span></div>
    <dialog bind:this={dialog} class="p-drawer" onclose={()=>trigger?.focus()} aria-labelledby="detail-title" aria-describedby="detail-description"><div class="p-drawer-inner"><header><span class="p-eyebrow">{incident?'Détail de l’incident':'Détail de la cible'}</span><button class="p-icon-btn" aria-label="Fermer le détail" onclick={()=>dialog.close()}><Icon name="close" size={20}/></button></header><span class={`p-badge ${selected.tone}`}><span class="p-dot"></span>{selected.state}</span><h2 id="detail-title">{selected.name}</h2><p id="detail-description">{incident?.title??'Les sources récentes confirment un état opérationnel.'}</p>{#if incident}<div class="p-detail-summary"><span>Gravité<strong class={incident.tone}>{incident.severity}</strong></span><span>Début<strong>{incident.since.replace('Depuis ','')}</strong></span><span>Assurance<strong>{incident.assurance}</strong></span></div><h3>Ce que les preuves établissent</h3><div class="p-evidence"><div><span class="p-source-symbol">{incident.source[0]}</span><strong>{incident.source}</strong><span class={`p-state ${incident.tone}`}><span class="p-dot"></span>Active</span></div><p>{incident.proof}</p><small>Observation du scénario à 14:32 · Source fraîche</small></div>{#if selected.id===0}<div class="p-detail-chart"><h3>Contexte au même instant</h3><Chart label="Évolution du temps de réponse de la cible"/><p>Indicateur contextuel. Le seuil et l’alerte appartiennent à Uptime Kuma.</p></div>{/if}<h3>Journal d’activité</h3><ol class="p-timeline">{#if acknowledged.includes(incident.id)}<li><span class="p-timeline-mark"></span><time>14:32</time><div><strong>Incident acquitté</strong><p>Pris en charge par Grégory · démonstration.</p></div></li>{/if}<li><span class="p-timeline-mark warn"></span><time>{incident.since.replace('Depuis ','')}</time><div><strong>Incident ouvert</strong><p>{incident.title}. Preuve reçue de {incident.source}.</p></div></li><li><span class="p-timeline-mark ok"></span><time>{selected.id===0?'14:17':'13:21'}</time><div><strong>Dernier état opérationnel</strong><p>Les observations précédentes étaient favorables.</p></div></li></ol><footer class="p-drawer-actions">{#if acknowledged.includes(incident.id)}<div class="p-ack">✓ Pris en charge · supervision toujours active</div>{:else}<button class="p-btn p-primary" onclick={()=>acknowledge(incident.id)}>Acquitter l’incident</button><p>L’acquittement confirme la prise en charge.</p>{/if}</footer>{:else}<div class="p-detail-summary"><span>Disponibilité<strong>{selected.availability} %</strong></span><span>Couverture<strong>{selected.coverage} %</strong></span><span>Latence<strong>{selected.latency} ms</strong></span></div><div class="p-evidence"><div><Icon name="health"/><strong>État opérationnel confirmé</strong></div><p>Aucun incident actif. Les dernières observations du scénario sont fraîches.</p></div>{/if}</div></dialog>
    {#if toast}<div class="p-toast" role="status"><span>✓</span>{toast}</div>{/if}
  </div>

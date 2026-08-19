/* État opérationnel partagé.
 *
 * Le passage à huit écrans routés sort cet état de la page unique : le shell et
 * chaque route lisent la même projection, alimentée par un seul flux temps réel.
 * Les Écrans supposent un rail toujours présent avec ses compteurs — l'état doit
 * donc survivre à la navigation. */

import {
  APIError,
  api,
  type Connector,
  type Incident,
  type Maintenance,
  type InboxEntry,
  type NotificationChannel,
  type Observation,
  type RealtimeMessage,
  type Source,
  type SystemHealth,
  type Target,
  type TargetMeasureDetail,
  type TargetMeasures,
  type User
} from './api';
import { i18n, t } from './i18n.svelte';

export type GateState = 'loading' | 'setup' | 'login' | 'unavailable' | 'app';
export type RealtimeState = 'connecting' | 'online' | 'offline';

export function messageFrom(cause: unknown): string {
  if (cause instanceof APIError) return cause.message;
  if (cause instanceof Error) return cause.message;
  return 'Erreur inattendue.';
}

class Session {
  gate = $state<GateState>('loading');
  health = $state<'checking' | 'ready' | 'unavailable'>('checking');
  version = $state('dev');
  user = $state<User | null>(null);

  /* Le nom que porte cette instance. Il est lu avant toute session, parce que
   * la porte d'entrée doit déjà dire où l'on frappe ; vide, il laisse le
   * produit se nommer lui-même. */
  instanceName = $state('');

  /* Combien de fois le compte courant est ouvert, cette session comprise. Les
   * Réglages le disent avant de proposer de refermer. */
  activeSessions = $state(0);

  targets = $state<Target[]>([]);
  connectors = $state<Connector[]>([]);
  incidents = $state<Incident[]>([]);
  maintenances = $state<Maintenance[]>([]);
  channels = $state<NotificationChannel[]>([]);

  /* Ce que ce compte a reçu. La boîte n'appartient qu'à lui : deux personnes
   * connectées à la même instance ne lisent pas la même chose. */
  inbox = $state<InboxEntry[]>([]);
  unread = $state(0);
  system = $state<SystemHealth | null>(null);

  /* Disponibilité, Couverture, latence et tendance sur 24 heures, par Cible.
   * Le serveur les établit sur des agrégats horaires : une liste de Cibles,
   * si longue soit-elle, ne coûte qu'une requête. */
  measures = $state<Record<string, TargetMeasures>>({});

  /* Les trois fenêtres et la part de chaque Source, chargées seulement par le
   * détail qui les montre. */
  measureDetails = $state<Record<string, TargetMeasureDetail>>({});

  /* Les Observations brutes d'une Cible, sous le Journal qui les résume. Elles
   * ne sont chargées que lorsqu'on demande à les voir : ce sont des milliers de
   * lignes par jour, et le Journal suffit à raconter l'Incident. */
  observations = $state<Record<string, Observation[]>>({});

  realtime = $state<RealtimeState>('offline');
  lastEventAt = $state<Date | null>(null);

  lightTheme = $state(false);
  identityBusy = $state(false);
  identityError = $state('');
  notice = $state('');

  #socket: WebSocket | null = null;
  #enabled = false;
  #attempt = 0;
  #cursor: number | null = null;
  #noticeTimer: ReturnType<typeof setTimeout> | undefined;
  #projectionTimer: ReturnType<typeof setTimeout> | undefined;
  #reconnectTimer: ReturnType<typeof setTimeout> | undefined;
  #refreshTimer: ReturnType<typeof setInterval> | undefined;
  #dirty = new Set<string>();

  /* ── Projections dérivées, lues par le rail et les écrans ─────────────── */

  /* Ce que les écrans affichent pour désigner l'instance. Les installations
   * mises en service avant qu'on puisse les nommer gardent « CairnOps ». */
  get instanceLabel() {
    return this.instanceName || 'CairnOps';
  }

  get actionable() {
    return this.incidents.filter((incident) => !incident.maintenance_active);
  }

  get unacknowledged() {
    return this.actionable.filter((incident) => !incident.acknowledged_at);
  }

  get activeMaintenances() {
    return this.maintenances.filter((maintenance) => maintenance.state === 'active');
  }

  get visibleMaintenances() {
    return this.maintenances.filter(
      (maintenance) => maintenance.state === 'active' || maintenance.state === 'upcoming'
    );
  }

  /** L'État de santé d'une Cible, déduit des Incidents qui la concernent.
   *  Une Divergence de Sources ne crée pas un cinquième État. */
  targetState(target: Target): 'down' | 'degraded' | 'maintenance' | 'unknown' | 'ok' {
    const own = this.incidents.filter((incident) => incident.target_id === target.id);
    if (own.some((incident) => incident.maintenance_active)) return 'maintenance';
    if (own.some((incident) => incident.effective_severity === 'critical')) return 'down';
    if (own.some((incident) => incident.effective_severity === 'major')) return 'down';
    if (own.some((incident) => incident.effective_severity === 'warning')) return 'degraded';
    if (own.some((incident) => incident.effective_severity === 'information')) return 'unknown';
    if (target.sources.length === 0 && target.external_source_count === 0) return 'unknown';
    return 'ok';
  }

  /** Une Cible dont les Sources ne concluent pas la même chose. */
  hasDivergence(target: Target): boolean {
    return this.incidents.some((incident) => {
      if (incident.target_id !== target.id) return false;
      const live = incident.signals.filter((signal) => !signal.invalidated_at);
      return live.some((signal) => signal.active) && live.some((signal) => !signal.active);
    });
  }

  incidentsFor(targetId: string) {
    return this.incidents.filter((incident) => incident.target_id === targetId);
  }

  showNotice(message: string) {
    this.notice = message;
    if (this.#noticeTimer) clearTimeout(this.#noticeTimer);
    this.#noticeTimer = setTimeout(() => (this.notice = ''), 6000);
  }

  /* ── Thème ────────────────────────────────────────────────────────────── */

  applyTheme() {
    document.documentElement.dataset.theme = this.lightTheme ? 'light' : 'dark';
  }

  toggleTheme() {
    this.lightTheme = !this.lightTheme;
    localStorage.setItem('cairnops-theme', this.lightTheme ? 'light' : 'dark');
    this.applyTheme();
  }

  /* ── Nom de l'instance ────────────────────────────────────────────────── */

  /* Renommer ne déplace aucune donnée : c'est l'étiquette que le rail, l'onglet
   * et la porte d'entrée lisent. L'état local ne bouge qu'une fois le serveur
   * d'accord, pour que deux écrans ouverts ne se contredisent pas. */
  async renameInstance(name: string) {
    const status = await api<{ initialized: boolean; name: string }>('/api/v1/instance', {
      method: 'PATCH',
      body: JSON.stringify({ name })
    });
    this.instanceName = status.name;
    return status.name;
  }

  /* ── Amorçage ─────────────────────────────────────────────────────────── */

  async boot() {
    /* La langue et le thème se posent avant tout appel : ce sont les deux
     * réglages que l'écran porte dès sa première image. */
    i18n.boot();
    this.lightTheme = localStorage.getItem('cairnops-theme') === 'light';
    this.applyTheme();
    await Promise.all([this.loadInfrastructure(), this.loadIdentity()]);
  }

  async loadInfrastructure() {
    try {
      const [ready, version] = await Promise.all([
        fetch('/api/v1/health/ready'),
        fetch('/api/v1/version')
      ]);
      this.health = ready.ok ? 'ready' : 'unavailable';
      if (version.ok) {
        const info: { version?: string } = await version.json();
        /* Une version vide vaut absente : `??` la laisserait passer et le rail
         * afficherait un « v » orphelin. */
        this.version = info.version || this.version;
      }
    } catch {
      this.health = 'unavailable';
    }
  }

  async loadIdentity() {
    this.identityError = '';
    try {
      const status = await api<{ initialized: boolean; name: string }>('/api/v1/setup/status');
      this.instanceName = status.name;
      if (!status.initialized) {
        this.gate = 'setup';
        return;
      }
      try {
        const session = await api<{ user: User; active_sessions: number }>('/api/v1/session');
        this.user = session.user;
        this.activeSessions = session.active_sessions;
        await this.enter();
      } catch (cause) {
        if (cause instanceof APIError && cause.status === 401) {
          this.gate = 'login';
          return;
        }
        throw cause;
      }
    } catch (cause) {
      this.identityError = messageFrom(cause);
      this.gate = 'unavailable';
    }
  }

  async setup(input: {
    bootstrap: string;
    instance_name: string;
    username: string;
    display_name: string;
    password: string;
  }) {
    this.identityBusy = true;
    this.identityError = '';
    try {
      const session = await api<{ user: User }>('/api/v1/setup', {
        method: 'POST',
        headers: { Authorization: `Bearer ${input.bootstrap}` },
        body: JSON.stringify({
          instance_name: input.instance_name,
          username: input.username,
          display_name: input.display_name,
          password: input.password
        })
      });
      this.user = session.user;
      this.instanceName = input.instance_name.trim();
      await this.enter();
      this.showNotice(t('session.commissioned'));
      return this.user;
    } catch (cause) {
      this.identityError = messageFrom(cause);
    } finally {
      this.identityBusy = false;
    }
  }

  async login(input: { username: string; password: string }) {
    this.identityBusy = true;
    this.identityError = '';
    try {
      const session = await api<{ user: User; active_sessions: number }>('/api/v1/session', {
        method: 'POST',
        body: JSON.stringify(input)
      });
      this.user = session.user;
      this.activeSessions = session.active_sessions;
      await this.enter();
      return this.user;
    } catch (cause) {
      this.identityError =
        cause instanceof APIError && cause.status === 401
          ? 'Identifiant ou mot de passe incorrect.'
          : messageFrom(cause);
    } finally {
      this.identityBusy = false;
    }
  }

  /* Porte de secours : aucune session n'existe encore, la preuve est le Jeton
   * d'amorçage porté par l'en-tête, exactement comme à la mise en service. */
  async recover(input: { bootstrap: string; username: string; password: string }) {
    this.identityBusy = true;
    this.identityError = '';
    try {
      await api<{ status: string }>('/api/v1/recovery', {
        method: 'POST',
        headers: { Authorization: `Bearer ${input.bootstrap}` },
        body: JSON.stringify({ username: input.username, new_password: input.password })
      });
      const user = await this.login({ username: input.username, password: input.password });
      if (user) this.showNotice(t('session.recovered'));
      return user;
    } catch (cause) {
      this.identityError =
        cause instanceof APIError && cause.status === 401
          ? t('session.badBootstrap')
          : messageFrom(cause);
    } finally {
      this.identityBusy = false;
    }
  }

  async logout() {
    try {
      await api<void>('/api/v1/session', { method: 'DELETE' });
    } catch {
      // La session locale est écartée même si le serveur l'a déjà expirée.
    } finally {
      this.stopRealtime();
      this.user = null;
      this.activeSessions = 0;
      this.targets = [];
      this.measures = {};
      this.measureDetails = {};
      this.connectors = [];
      this.incidents = [];
      this.maintenances = [];
      this.channels = [];
      this.inbox = [];
      this.unread = 0;
      this.system = null;
      this.#cursor = null;
      this.gate = 'login';
    }
  }

  async enter() {
    this.gate = 'app';
    await this.refreshAll();
    if (this.gate !== 'app' || !this.user) return;
    this.startRealtime();
    if (this.#refreshTimer) clearInterval(this.#refreshTimer);
    this.#refreshTimer = setInterval(() => {
      void this.loadSystemHealth();
      void this.loadIncidents();
      void this.loadMaintenances();
      void this.loadNotifications();
      void this.loadInbox();
      /* Les mesures suivent le battement de quinze secondes plutôt que chaque
       * Observation : une latence moyenne sur 24 heures ne bouge pas à la
       * seconde, et une liste de Cibles n'a pas à recharger si souvent. */
      void this.loadMeasures();
      for (const targetId of Object.keys(this.measureDetails)) {
        void this.loadMeasureDetail(targetId);
      }
    }, 15000);
  }

  async refreshAll() {
    await Promise.all([
      this.loadTargets(),
      this.loadMeasures(),
      this.loadConnectors(),
      this.loadIncidents(),
      this.loadMaintenances(),
      this.loadNotifications(),
      this.loadInbox(),
      this.loadSystemHealth()
    ]);
  }

  /* ── Chargements ──────────────────────────────────────────────────────── */

  /** Toute réponse 401 renvoie à la porte de connexion sans effacer la
   *  projection précédente : un écran vide vaut moins qu'un écran daté. */
  #expired(cause: unknown): boolean {
    if (cause instanceof APIError && cause.status === 401) {
      this.stopRealtime();
      this.user = null;
      this.activeSessions = 0;
      this.gate = 'login';
      return true;
    }
    return false;
  }

  async loadTargets() {
    try {
      const response = await api<{ targets: Target[] }>('/api/v1/targets');
      this.targets = response.targets;
    } catch (cause) {
      if (this.#expired(cause)) return;
      this.showNotice(t('session.refreshTargets', { error: messageFrom(cause) }));
    }
  }

  async loadSystemHealth() {
    try {
      this.system = await api<SystemHealth>('/api/v1/system/health');
    } catch (cause) {
      if (this.#expired(cause)) return;
      this.system = null;
    }
  }

  async loadConnectors() {
    try {
      const response = await api<{ connectors: Connector[] }>('/api/v1/connectors');
      this.connectors = response.connectors;
    } catch (cause) {
      if (this.#expired(cause)) return;
      this.showNotice(t('session.refreshConnectors', { error: messageFrom(cause) }));
    }
  }

  async loadIncidents() {
    try {
      const response = await api<{ incidents: Incident[] }>('/api/v1/incidents?status=active&limit=200');
      this.incidents = response.incidents;
    } catch (cause) {
      if (this.#expired(cause)) return;
      this.showNotice(t('session.refreshIncidents', { error: messageFrom(cause) }));
    }
  }

  async loadMaintenances() {
    try {
      const response = await api<{ maintenances: Maintenance[] }>('/api/v1/maintenances?limit=100');
      this.maintenances = response.maintenances;
    } catch (cause) {
      if (this.#expired(cause)) return;
      this.showNotice(t('session.refreshMaintenances', { error: messageFrom(cause) }));
    }
  }

  async loadNotifications() {
    if (this.user?.role !== 'administrator') {
      this.channels = [];
      return;
    }
    try {
      const response = await api<{ channels: NotificationChannel[] }>('/api/v1/notification-channels');
      this.channels = response.channels;
    } catch (cause) {
      if (this.#expired(cause)) return;
      this.showNotice(t('session.refreshNotifications', { error: messageFrom(cause) }));
    }
  }

  /* La boîte se lit quel que soit le rôle : les Canaux sont une administration,
   * les notifications reçues ne le sont pas. */
  async loadInbox() {
    try {
      const response = await api<{ entries: InboxEntry[]; unread: number }>('/api/v1/notifications');
      this.inbox = response.entries;
      this.unread = response.unread;
    } catch (cause) {
      if (this.#expired(cause)) return;
      /* Une boîte illisible ne mérite pas d'interrompre la veille : le rail
       * gardera son dernier compte connu jusqu'au prochain battement. */
    }
  }

  /* Marquer lu ne concerne que soi, et n'efface rien : l'entrée reste dans la
   * boîte avec sa date. */
  async markInboxRead(ids?: number[]) {
    const targeted = ids ?? this.inbox.filter((entry) => !entry.read_at).map((entry) => entry.id);
    if (targeted.length === 0) return;
    const now = new Date().toISOString();
    this.inbox = this.inbox.map((entry) =>
      targeted.includes(entry.id) && !entry.read_at ? { ...entry, read_at: now } : entry
    );
    this.unread = Math.max(0, this.unread - targeted.length);
    try {
      await api<{ read: number }>('/api/v1/notifications/read', {
        method: 'POST',
        body: JSON.stringify({ ids: targeted })
      });
    } catch (cause) {
      if (this.#expired(cause)) return;
      /* La lecture optimiste a menti : la boîte relue remet l'écran d'accord
       * avec l'instance. */
      await this.loadInbox();
    }
  }

  async loadMeasures() {
    try {
      const response = await api<{ targets: TargetMeasures[] }>('/api/v1/metrics/targets');
      this.measures = Object.fromEntries(response.targets.map((measured) => [measured.target_id, measured]));
    } catch (cause) {
      if (this.#expired(cause)) return;
      /* Une mesure manquante s'affiche comme absente ; elle ne vide pas
       * l'écran ni ne déclenche de notice. */
    }
  }

  /** Les trois fenêtres d'une Cible, pour l'écran qui les détaille. */
  async loadMeasureDetail(targetId: string) {
    try {
      const detail = await api<TargetMeasureDetail>(`/api/v1/targets/${targetId}/metrics`);
      this.measureDetails = { ...this.measureDetails, [targetId]: detail };
    } catch (cause) {
      if (this.#expired(cause)) return;
    }
  }

  /** Les dernières Observations d'une Cible, telles qu'elles ont été écrites. */
  async loadObservations(targetId: string) {
    try {
      const response = await api<{ observations: Observation[] }>(
        `/api/v1/targets/${targetId}/observations?limit=100`
      );
      this.observations = { ...this.observations, [targetId]: response.observations };
    } catch (cause) {
      if (this.#expired(cause)) return;
      this.showNotice(`Impossible de lire les Observations : ${messageFrom(cause)}`);
    }
  }

  measuresFor(targetId: string): TargetMeasures | null {
    return this.measures[targetId] ?? null;
  }

  /* ── Temps réel ───────────────────────────────────────────────────────── */

  startRealtime() {
    this.#enabled = true;
    this.#connect();
  }

  #connect() {
    if (!this.#enabled || this.gate !== 'app' || this.#socket) return;
    if (this.#reconnectTimer) clearTimeout(this.#reconnectTimer);
    this.realtime = 'connecting';

    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const cursor = this.#cursor === null ? '' : `?after=${this.#cursor}`;
    const socket = new WebSocket(`${protocol}//${location.host}/api/v1/events${cursor}`);
    this.#socket = socket;

    socket.onmessage = (event) => {
      try {
        const message = JSON.parse(String(event.data)) as RealtimeMessage;
        if ((message.type !== 'ready' && message.type !== 'event') || !Number.isSafeInteger(message.version)) return;
        this.#cursor = Math.max(this.#cursor ?? 0, message.version);
        this.realtime = 'online';
        this.lastEventAt = new Date();
        this.#attempt = 0;
        if (message.type === 'event') this.#scheduleRefresh(message.kind);
      } catch {
        // Un message inconnu n'invalide pas la dernière projection valable.
      }
    };
    socket.onerror = () => socket.close();
    socket.onclose = () => {
      if (this.#socket === socket) this.#socket = null;
      if (!this.#enabled) return;
      this.realtime = 'offline';
      const delay = Math.min(1000 * 2 ** this.#attempt, 10000);
      this.#attempt += 1;
      this.#reconnectTimer = setTimeout(() => this.#connect(), delay);
    };
  }

  stopRealtime() {
    this.#enabled = false;
    this.realtime = 'offline';
    if (this.#reconnectTimer) clearTimeout(this.#reconnectTimer);
    this.#reconnectTimer = undefined;
    if (this.#refreshTimer) clearInterval(this.#refreshTimer);
    this.#refreshTimer = undefined;
    const socket = this.#socket;
    this.#socket = null;
    if (socket && socket.readyState < WebSocket.CLOSING) socket.close(1000, 'session closed');
  }

  #scheduleRefresh(kind: RealtimeMessage['kind']) {
    if (kind === 'component.heartbeat') this.#dirty.add('health');
    else if (kind === 'connector.changed') this.#dirty.add('connectors');
    else if (kind === 'incident.changed') this.#dirty.add('incidents');
    else if (kind === 'maintenance.changed') {
      this.#dirty.add('maintenances');
      this.#dirty.add('incidents');
    } else if (kind === 'notification.changed') this.#dirty.add('notifications');
    else this.#dirty.add('targets');

    if (this.#projectionTimer) clearTimeout(this.#projectionTimer);
    this.#projectionTimer = setTimeout(() => {
      if (this.#dirty.has('health')) void this.loadSystemHealth();
      if (this.#dirty.has('targets')) void this.loadTargets();
      if (this.#dirty.has('connectors')) void this.loadConnectors();
      if (this.#dirty.has('incidents')) void this.loadIncidents();
      if (this.#dirty.has('maintenances')) void this.loadMaintenances();
      if (this.#dirty.has('notifications')) {
        void this.loadNotifications();
        void this.loadInbox();
      }
      this.#dirty.clear();
    }, 90);
  }

  teardown() {
    this.stopRealtime();
    if (this.#noticeTimer) clearTimeout(this.#noticeTimer);
    if (this.#projectionTimer) clearTimeout(this.#projectionTimer);
  }

  /* ── Actions ──────────────────────────────────────────────────────────── */

  async acknowledge(incident: Incident) {
    try {
      const acknowledged = await api<Incident>(`/api/v1/incidents/${incident.id}/acknowledgement`, {
        method: 'POST'
      });
      this.incidents = this.incidents.map((item) => (item.id === acknowledged.id ? acknowledged : item));
      this.showNotice(
        acknowledged.acknowledgement_sync_status === 'failed'
          ? t('session.acknowledgedUnsynced')
          : t('session.acknowledged')
      );
    } catch (cause) {
      this.showNotice(t('session.acknowledgeFailed', { error: messageFrom(cause) }));
    }
  }

  async invalidate(incidentId: string, signalId: string, reason: string) {
    try {
      await api<Incident>(`/api/v1/incidents/${incidentId}/signals/${signalId}/invalidation`, {
        method: 'POST',
        body: JSON.stringify({ reason })
      });
      await this.loadIncidents();
      this.showNotice(t('session.invalidated'));
      return true;
    } catch (cause) {
      this.showNotice(t('session.invalidateFailed', { error: messageFrom(cause) }));
      return false;
    }
  }

  /* Suspendre n'efface rien : la lecture s'arrête, les liaisons et les Incidents
   * déjà ouverts restent tels quels. Reprendre remet le Connecteur dû
   * immédiatement, si bien que son état réel revient au premier cycle. */
  async setConnectorSuspension(connector: Connector) {
    const suspend = connector.status !== 'disabled';
    try {
      const updated = await api<Connector>(`/api/v1/connectors/${connector.id}/suspension`, {
        method: suspend ? 'POST' : 'DELETE'
      });
      this.connectors = this.connectors.map((item) => (item.id === updated.id ? updated : item));
      this.showNotice(
        suspend
          ? t('session.connectorSuspended', { name: updated.name })
          : t('session.connectorResumed', { name: updated.name })
      );
    } catch (cause) {
      this.showNotice(
        `Impossible de ${suspend ? 'suspendre' : 'reprendre'} le Connecteur : ${messageFrom(cause)}`
      );
    }
  }

  /* Corriger une Cible ne change ni son identité ni son histoire : la
   * projection est rechargée, jamais reconstruite. */
  async renameTarget(targetId: string, name: string, description: string) {
    try {
      await api<Target>(`/api/v1/targets/${targetId}`, {
        method: 'PATCH',
        body: JSON.stringify({ name, description })
      });
      await this.loadTargets();
      this.showNotice(t('session.targetRenamed'));
      return true;
    } catch (cause) {
      this.showNotice(`Impossible de renommer la Cible : ${messageFrom(cause)}`);
      return false;
    }
  }

  /** Archiver retire la Cible du service sans effacer son passé. */
  async archiveTarget(targetId: string) {
    try {
      await api<void>(`/api/v1/targets/${targetId}`, { method: 'DELETE' });
      await Promise.all([this.loadTargets(), this.loadIncidents(), this.loadMeasures()]);
      this.showNotice(t('session.targetArchived'));
      return true;
    } catch (cause) {
      this.showNotice(t('session.archiveFailed', { error: messageFrom(cause) }));
      return false;
    }
  }

  async updateSource(sourceId: string, input: Record<string, unknown>) {
    try {
      await api<Source>(`/api/v1/sources/${sourceId}`, {
        method: 'PATCH',
        body: JSON.stringify(input)
      });
      await this.loadTargets();
      return true;
    } catch (cause) {
      this.showNotice(t('session.sourceUpdateFailed', { error: messageFrom(cause) }));
      return false;
    }
  }

  async deleteSource(sourceId: string) {
    try {
      await api<void>(`/api/v1/sources/${sourceId}`, { method: 'DELETE' });
      await Promise.all([this.loadTargets(), this.loadMeasures()]);
      this.showNotice(t('session.sourceRemoved'));
      return true;
    } catch (cause) {
      this.showNotice(t('session.sourceRemoveFailed', { error: messageFrom(cause) }));
      return false;
    }
  }

  async cancelMaintenance(maintenance: Maintenance) {
    try {
      await api<Maintenance>(`/api/v1/maintenances/${maintenance.id}/cancellation`, { method: 'POST' });
      await Promise.all([this.loadMaintenances(), this.loadIncidents()]);
      this.showNotice(t('session.maintenanceStopped'));
    } catch (cause) {
      this.showNotice(t('session.maintenanceStopFailed', { error: messageFrom(cause) }));
    }
  }
}

export const session = new Session();

import { t } from './i18n.svelte';

export type Role = 'administrator' | 'operator' | 'observer';

export type User = {
  id: string;
  username: string;
  display_name: string;
  role: Role;
};

/* Un compte vu depuis l'écran qui les administre. La session, elle, n'en connaît
 * que l'identité : created_at et deactivated_at ne servent qu'ici. */
export type Account = User & {
  created_at: string;
  deactivated_at: string | null;
  /* La présence du compte : combien de sessions il tient ouvertes, et quand il
   * s'est manifesté pour la dernière fois. Jamais venu, jamais vu : null. */
  active_sessions: number;
  last_seen_at: string | null;
};

export type DevicePlatform = 'ios' | 'android';
export type DeviceNotificationContent = 'complete' | 'discreet' | 'masked';

export type Device = {
  id: string;
  user_id: string;
  user_display_name: string;
  name: string;
  platform: DevicePlatform;
  app_version: string;
  locale: 'fr' | 'en';
  notification_content: DeviceNotificationContent;
  push_enabled: boolean;
  last_seen_at: string | null;
  revoked_at: string | null;
  created_at: string;
  updated_at: string;
};

export type DevicePairingStatus =
  | 'awaiting_scan'
  | 'awaiting_confirmation'
  | 'confirmed'
  | 'credential_consumed'
  | 'expired'
  | 'cancelled';

export type DevicePairing = {
  id: string;
  status: DevicePairingStatus;
  expires_at: string;
  claimed_name?: string;
  claimed_platform?: DevicePlatform;
  claimed_at?: string;
  confirmed_at?: string;
  device_id?: string;
  created_at: string;
};

export type DevicePairingInvitation = {
  pairing: DevicePairing;
  instance_url: string;
  token: string;
  qr_payload: string;
};

export type SourceKind = 'http' | 'tcp' | 'dns' | 'icmp' | 'heartbeat';
export type Outcome = 'healthy' | 'unhealthy' | 'unknown';

export type Source = {
  id: string;
  target_id: string;
  name: string;
  kind: SourceKind;
  enabled: boolean;
  interval_seconds: number;
  timeout_milliseconds: number;
  failure_threshold: number;
  recovery_threshold: number;
  severity: IncidentSeverity;
  last_signal_at?: string;
  last_observed_at?: string;
  latest_outcome?: Outcome;
};

export type Observation = {
  id: number;
  source_id: string;
  source_name: string;
  observed_at: string;
  outcome: Outcome;
  latency_milliseconds: number;
  reason?: string;
  message?: string;
  details: Record<string, unknown>;
};

export type Target = {
  id: string;
  name: string;
  description: string;
  created_at: string;
  external_source_count: number;
  sources: Source[];
};

export type CreatedSource = {
  source: Source;
  heartbeat_token?: string;
  heartbeat_path?: string;
};

/* ── Mesures ──────────────────────────────────────────────────────────────
 * Disponibilité, Couverture et latence viennent du serveur, qui les lit sur
 * des agrégats horaires. Une mesure absente reste `null` : l'interface montre
 * alors un tiret plutôt qu'un zéro rassurant. */

export type MeasureWindow = '24h' | '7d' | '30d';

export type Measure = {
  window: MeasureWindow;
  availability: number | null;
  coverage: number | null;
  average_latency_milliseconds: number | null;
  maximum_latency_milliseconds: number | null;
  conclusive_observations: number;
  unknown_observations: number;
  expected_observations: number;
};

export type TargetMeasures = {
  target_id: string;
  measures: Measure[];
  trend: number[];
  latency_trend: number[];
  latest_observed_at?: string;
  sources: SourceMeasures[];
};

export type SourceMeasures = {
  source_id: string;
  name: string;
  kind: SourceKind | 'zabbix' | 'uptime_kuma' | 'patchmon' | 'generic_webhook';
  origin: 'native' | 'integration';
  measures_availability: boolean;
  latest_outcome?: Outcome;
  latest_observed_at?: string;
  measures: Measure[];
};

export type TargetMeasureDetail = {
  target_id: string;
  generated_at: string;
  measures: Measure[];
  trend: number[];
  latency_trend: number[];
  latest_observed_at?: string;
  sources: SourceMeasures[];
};

export type ComponentStatus = 'operational' | 'stale' | 'unavailable';

export type SystemComponent = {
  name: 'server' | 'worker' | 'postgresql' | 'push';
  status: ComponentStatus;
  instances: number;
  last_seen_at?: string;
  detail?: string;
};

/* Temps de réponse de PostgreSQL, mesuré à chaque lecture de la Santé. Les
 * mesures vivent en mémoire côté serveur : elles disent l'instant, pas
 * l'historique, et un redémarrage a le droit de les oublier. */
export type DatabaseHealth = {
  latency_milliseconds: number;
  maximum_latency_milliseconds: number;
  samples: number[];
  measured_since: string;
};

/* L'activité de toute l'instance sur une heure : ce que les Contrôles
 * devaient exécuter, ce qu'ils ont conclu, et à quelle vitesse. */
export type InstanceHour = {
  hour: string;
  expected_observations: number;
  conclusive_observations: number;
  healthy_observations: number;
  average_latency_milliseconds: number | null;
};

export type SystemHealth = {
  status: 'operational' | 'degraded';
  checked_at: string;
  components: SystemComponent[];
  database: DatabaseHealth;
  hours: InstanceHour[];
};

export type Connector = {
  id: string;
  kind: 'zabbix' | 'uptime_kuma' | 'patchmon' | 'generic_webhook';
  name: string;
  endpoint: string;
  status: 'connected' | 'degraded' | 'disabled';
  remote_version: string;
  compatibility: 'supported' | 'warning';
  encrypted_transport: boolean;
  binding_count: number;
  quarantine_count: number;
  last_checked_at: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
};

export type IndicatorSemanticKey =
  | 'cpu.utilization'
  | 'memory.utilization'
  | 'filesystem.utilization'
  | 'network.in'
  | 'network.out'
  | 'response.time'
  | 'certificate.days_remaining'
  | 'certificate.valid'
  | 'updates.count'
  | 'security_updates.count'
  | 'reboot.required'
  | 'reporting.age';

export type IndicatorUnit =
  | 'percent'
  | 'bytes_per_second'
  | 'milliseconds'
  | 'days'
  | 'count'
  | 'boolean'
  | 'seconds';

export type IndicatorCandidate = {
  semantic_key: IndicatorSemanticKey;
  label: string;
  external_id: string;
  dimension?: string;
  unit: IndicatorUnit;
  recommended: boolean;
  available: boolean;
  reason?: string;
  metadata: Record<string, unknown>;
};

export type ContextIndicator = {
  id: string;
  connector_id: string;
  binding_id: string;
  target_id: string;
  semantic_key: IndicatorSemanticKey;
  label: string;
  external_id: string;
  dimension?: string;
  unit: IndicatorUnit;
  enabled: boolean;
  metadata: Record<string, unknown>;
  last_value?: number;
  last_observed_at?: string;
  last_error?: string;
  pinned: boolean;
  pin_position?: number;
};

export type IndicatorBinding = {
  id?: string;
  target_id?: string;
  target_name?: string;
  external_id: string;
  external_name: string;
  enabled: boolean;
  imported: boolean;
  indicators: ContextIndicator[];
  candidates: IndicatorCandidate[];
};

export type IndicatorProfileEntry = {
  semantic_key: IndicatorSemanticKey;
  dimension?: string;
  enabled: boolean;
};

export type IndicatorProfile = {
  id: string;
  name: string;
  specification: IndicatorProfileEntry[];
  created_at: string;
  updated_at: string;
};

export type IndicatorConfiguration = {
  connector_id: string;
  connector_kind: Exclude<Connector['kind'], 'generic_webhook'>;
  connector_name: string;
  endpoint: string;
  generated_at: string;
  expires_at?: string;
  capabilities: Array<{
    key: string;
    status: 'available' | 'degraded' | 'unavailable';
    message?: string;
    checked_at: string;
  }>;
  bindings: IndicatorBinding[];
  profiles: IndicatorProfile[];
  activity: Array<{
    id: number;
    actor_id?: string;
    actor_name?: string;
    summary: string;
    data: Record<string, unknown>;
    occurred_at: string;
  }>;
};

export type IndicatorPoint = {
  at: string;
  value: number;
  minimum?: number;
  maximum?: number;
  samples?: number;
};

export type TargetIndicators = {
  target_id: string;
  generated_at: string;
  indicators: ContextIndicator[];
  series?: Record<string, IndicatorPoint[]>;
};

export type IncidentIndicators = {
  incident_id: string;
  target_id: string;
  opened_at: string;
  snapshots: Array<{
    indicator_id?: string;
    semantic_key: IndicatorSemanticKey;
    label: string;
    unit: IndicatorUnit;
    value: number;
    observed_at: string;
  }>;
  indicators: ContextIndicator[];
  series: Record<string, IndicatorPoint[]>;
  disclaimer: string;
};

/* Le compte rendu de suppression vient du serveur : c'est lui qui sait ce que
 * la cascade a réellement emporté, l'écran ne faisait qu'une estimation. */
export type ConnectorRemoval = {
  id: string;
  kind: Connector['kind'];
  name: string;
  bindings: number;
  quarantined: number;
  resolved_incidents: number;
};

export type ZabbixInterface = {
  type: string;
  address: string;
  port?: string;
  main: boolean;
};

export type TargetReference = { id: string; name: string };

export type MatchEvidence = {
  kind: 'same_machine_id' | 'same_name' | 'same_ip' | 'same_hostname' | 'similar_name';
  value: string;
};

export type TargetMatch = {
  target: TargetReference;
  confidence: 'high' | 'medium' | 'low';
  evidence: MatchEvidence[];
};

export type ZabbixHostPreview = {
  external_id: string;
  name: string;
  technical_name: string;
  interfaces: ZabbixInterface[];
  candidate_targets?: TargetMatch[];
  suggested_target?: TargetReference;
  already_imported_to?: TargetReference;
};

export type ZabbixPreview = {
  kind: 'zabbix';
  name: string;
  endpoint: string;
  version: string;
  compatibility: 'supported' | 'warning';
  compatibility_label: string;
  encrypted_transport: boolean;
  hosts: ZabbixHostPreview[];
  available_targets: TargetReference[];
  receipt: string;
  expires_at: string;
};

export type ConnectorImportResult = {
  connector: Connector;
  targets: Array<{
    external_id: string;
    target_id: string;
    target_name: string;
    disposition: 'created' | 'reused' | 'already_imported';
  }>;
};

export type ZabbixImportResult = ConnectorImportResult;

export type UptimeKumaMonitorPreview = {
  external_id: string;
  name: string;
  type: string;
  address?: string;
  status: 0 | 1 | 2 | 3;
  candidate_targets?: TargetMatch[];
  suggested_target?: TargetReference;
  already_imported_to?: TargetReference;
};

export type UptimeKumaPreview = {
  kind: 'uptime_kuma';
  name: string;
  endpoint: string;
  compatibility: 'supported';
  compatibility_label: string;
  encrypted_transport: boolean;
  monitors: UptimeKumaMonitorPreview[];
  available_targets: TargetReference[];
  receipt: string;
  expires_at: string;
};

export type UptimeKumaImportResult = ConnectorImportResult;

export type PatchMonHostPreview = {
  external_id: string;
  name: string;
  hostname: string;
  ip?: string;
  os_type?: string;
  os_version?: string;
  reporting_state?: string;
  update_state?: string;
  updates_count: number;
  security_updates_count: number;
  needs_reboot: boolean;
  candidate_targets?: TargetMatch[];
  suggested_target?: TargetReference;
  already_imported_to?: TargetReference;
};

export type PatchMonPreview = {
  kind: 'patchmon';
  name: string;
  endpoint: string;
  compatibility: 'supported';
  compatibility_label: string;
  encrypted_transport: boolean;
  hosts: PatchMonHostPreview[];
  available_targets: TargetReference[];
  receipt: string;
  expires_at: string;
};

export type PatchMonImportResult = ConnectorImportResult;

export type GenericWebhookCreated = {
  connector: Connector;
  endpoint: string;
  token: string;
};

export type WebhookQuarantine = {
  id: string;
  connector_id: string;
  external_identity: string;
  target_name: string;
  event_key: string;
  nature_key: string;
  nature: string;
  status: 'firing' | 'resolved';
  severity: IncidentSeverity;
  summary: string;
  details: Record<string, unknown>;
  occurrences: number;
  first_seen_at: string;
  last_seen_at: string;
};

export type WebhookApproval = {
  target_id: string;
  target_name: string;
  identity: string;
  replayed: number;
};

export type IncidentSeverity = 'information' | 'warning' | 'major' | 'critical';

export type IncidentSignal = {
  id: string;
  origin: 'zabbix' | 'uptime_kuma' | 'patchmon' | 'webhook' | 'native';
  connector_id?: string;
  connector_name?: string;
  external_event_id?: string;
  external_object_id?: string;
  name: string;
  active: boolean;
  severity: IncidentSeverity;
  opened_at: string;
  resolved_at?: string;
  upstream_acknowledged: boolean;
  invalidated_at?: string;
  invalidated_by?: string;
  invalidation_reason?: string;
  rearmed_at?: string;
};

export type IncidentActivity = {
  id: number;
  kind: string;
  origin: string;
  actor_name?: string;
  message: string;
  data: Record<string, unknown>;
  occurred_at: string;
};

export type Incident = {
  id: string;
  target_id: string;
  target_name: string;
  nature_key: string;
  nature_label: string;
  status: 'active' | 'resolved';
  source_severity: IncidentSeverity;
  effective_severity: IncidentSeverity;
  opened_at: string;
  resolved_at?: string;
  acknowledged_at?: string;
  acknowledged_by?: string;
  acknowledgement_origin?: 'user' | 'connector';
  acknowledgement_sync_status: 'not_applicable' | 'pending' | 'synchronized' | 'failed';
  acknowledgement_sync_error?: string;
  maintenance_active: boolean;
  maintenance_ends_at?: string;
  signals: IncidentSignal[];
  activity: IncidentActivity[];
  created_at: string;
  updated_at: string;
};

export type Maintenance = {
  id: string;
  name: string;
  reason: string;
  state: 'active' | 'upcoming' | 'ended' | 'cancelled';
  starts_at: string;
  ends_at: string;
  cancelled_at?: string;
  created_by?: string;
  targets: Array<{ id: string; name: string }>;
  created_at: string;
};

export type NotificationChannel = {
  id: string;
  kind: 'mattermost' | 'in_app';
  name: string;
  endpoint: string;
  severities: IncidentSeverity[];
  enabled: boolean;
  status: 'connected' | 'degraded' | 'disabled';
  encrypted_transport: boolean;
  last_checked_at: string;
  last_error?: string;
  created_at: string;
  updated_at: string;
};

/* Un jour de la fenêtre d'Incidents : combien s'y sont ouverts. Le serveur
 * rend chaque jour de la fenêtre, y compris ceux restés vides — une série
 * creuse dessinerait un passé plus calme qu'il ne fut. */
export type IncidentDay = {
  day: string;
  opened: number;
};

/* Une entrée de la boîte de réception. Elle appartient à une personne : le
 * serveur ne rend jamais que celle de la session qui demande. */
export type InboxEntry = {
  id: number;
  incident_id: string;
  target_id?: string;
  event_kind: 'firing' | 'resolved';
  target_name: string;
  nature_label: string;
  severity: IncidentSeverity;
  occurred_at: string;
  read_at: string | null;
};

export type RealtimeMessage = {
  type: 'ready' | 'event';
  version: number;
  kind?: 'target.changed' | 'source.changed' | 'observation.created' | 'component.heartbeat' | 'connector.changed' | 'incident.changed' | 'maintenance.changed' | 'notification.changed' | 'device.changed' | 'indicator.changed';
  entity_type?: 'target' | 'source' | 'component' | 'connector' | 'incident' | 'maintenance' | 'notification' | 'device' | 'indicator';
  entity_id?: string;
  occurred_at?: string;
};

export class APIError extends Error {
  constructor(
    message: string,
    readonly status: number
  ) {
    super(message);
  }
}

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    ...options,
    headers: {
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
      ...options.headers
    }
  });

  if (!response.ok) {
    let message = t('common.requestFailed', { status: response.status });
    try {
      const payload: { error?: string } = await response.json();
      if (payload.error) message = payload.error;
    } catch {
      // The status remains the useful fallback when a proxy returns a non-JSON error.
    }
    throw new APIError(message, response.status);
  }

  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

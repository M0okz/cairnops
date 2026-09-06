import type { CairnOpsIconName } from './brand/cairnops-icon-paths';

export type TimelineEntry = {
  id: string | number;
  occurred_at: string;
  kind: string;
  message: string;
  origin: string;
  actor_name?: string;
  context?: string;
};

/** Group by the reader's calendar day, keeping every fact and stable ties. */
export function activityDays<T extends Pick<TimelineEntry, 'occurred_at'>>(
  entries: T[], timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone
): { key: string; at: string; entries: T[] }[] {
  const calendar = new Intl.DateTimeFormat('en-CA', {
    timeZone, year: 'numeric', month: '2-digit', day: '2-digit'
  });
  const days = new Map<string, { key: string; at: string; entries: T[] }>();
  for (const entry of [...entries].sort((a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at))) {
    const key = calendar.format(new Date(entry.occurred_at));
    const day = days.get(key) ?? { key, at: entry.occurred_at, entries: [] };
    day.entries.push(entry);
    days.set(key, day);
  }
  return [...days.values()];
}

export function activityMarker(kind: string): { icon: CairnOpsIconName; restored: boolean } {
  const icons: Record<string, CairnOpsIconName> = {
    opened: 'incidents', impact_joined: 'plus', impact_reopened: 'worker',
    impact_resolved: 'state-healthy', resolved: 'state-healthy', evidence_resolved: 'health',
    evidence_added: 'signal', evidence_updated: 'signal', invalidated: 'close',
    propagation_closed: 'lock', extended: 'signal', severity_changed: 'incidents',
    acknowledged: 'acknowledge', upstream_acknowledged: 'acknowledge',
    ack_sync_succeeded: 'acknowledge', ack_sync_failed: 'state-degraded',
    target_reconciled: 'targets', source_reassigned: 'connectors'
  };
  return { icon: icons[kind] ?? 'activity', restored: ['resolved', 'impact_resolved', 'evidence_resolved'].includes(kind) };
}

export function activityOrigin(origin: string): string {
  return ({ cairnops: 'CairnOps', native: 'CairnOps', zabbix: 'Zabbix', uptime_kuma: 'Uptime Kuma',
    patchmon: 'PatchMon', argus: 'Argus', proxmox: 'Proxmox VE', webhook: 'Webhook' } as Record<string, string>)[origin] ?? origin;
}

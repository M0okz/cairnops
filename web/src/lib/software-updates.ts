export type ReleaseSource = {
  kind: "github" | "forgejo" | "gitlab" | "changelog";
  url: string;
  software: string;
};
export type ReleaseNote = {
  version: string;
  url: string;
  body: string;
  missing: boolean;
};
export type ReleasePoint = {
  category: "feature" | "fix" | "security" | "impact" | "upgrade";
  text: string;
  version: string;
  quote: string;
};
export type ReleaseAnalysis = {
  notes: ReleaseNote[];
  id: number;
  revision: number;
  installed_version: string;
  target_version: string;
  source: ReleaseSource;
  result: { overview: ReleasePoint[]; details: ReleasePoint[] };
  model: string;
  created_at: string;
  current: boolean;
};
export type UpdateGroup = "apply" | "review" | "current";
export type UpdateSituation =
  | "update"
  | "prerelease"
  | "target_older"
  | "unordered"
  | "current"
  | "unknown";
export type UpdateLevel = "major" | "minor" | "patch";
export type VerificationIssue =
  | "argus_unavailable"
  | "installed_unreadable"
  | "target_unreadable"
  | "service_missing"
  | "service_inactive"
  | "unconfirmed";
export type VersionEvent = {
  kind: "first" | "installed" | "target";
  version: string;
  previous?: string;
  target?: string;
  direction?: "upgrade" | "rollback" | "changed";
  observed_at: string;
};
export type SoftwareService = {
  id: string;
  target_id: string;
  resource_name: string;
  name: string;
  installed_version: string;
  target_version: string;
  observed_at: string | null;
  known: boolean;
  situation: UpdateSituation;
  level?: UpdateLevel;
  group: UpdateGroup;
  verification_issue?: VerificationIssue;
  approved: boolean;
  skipped: boolean;
  security_mentioned: boolean;
  source: ReleaseSource;
  source_origin: "argus" | "manual";
  suggested_source: ReleaseSource | null;
  confirmed_at: string | null;
  revision: number;
  state: string;
  last_error: string;
  checked_at: string | null;
  next_check_at: string | null;
  collection: {
    installed_version: string;
    target_version: string;
    notes: ReleaseNote[];
    incomplete: boolean;
  } | null;
  collection_revision: number | null;
  analyses: ReleaseAnalysis[];
  history: {
    installed_version: string;
    target_version: string;
    observed_at: string;
  }[];
  events: VersionEvent[];
};
export type SoftwareAIConfig = {
  enabled: boolean;
  endpoint: string;
  model: string;
  key_configured: boolean;
  api_key?: string;
};
export function safeReleaseURL(raw: string): string | undefined {
  try {
    const url = new URL(raw);
    return url.protocol === "https:" && !url.username && !url.password
      ? url.href
      : undefined;
  } catch {
    return undefined;
  }
}
export function currentAnalysis(
  service: SoftwareService,
): ReleaseAnalysis | undefined {
  return service.analyses.find(
    (a) =>
      a.current &&
      a.revision === service.revision &&
      a.installed_version === service.installed_version &&
      a.target_version === service.target_version,
  );
}

const levelRank: Record<UpdateLevel, number> = { major: 0, minor: 1, patch: 2 };
const situationRank: Partial<Record<UpdateSituation, number>> = {
  unknown: 0,
  update: 1,
  prerelease: 2,
  target_older: 3,
  unordered: 4,
};
export const updateGroups: UpdateGroup[] = ["apply", "review", "current"];

/** Le libellé principal d'un service est le logiciel suivi : une même
 *  Ressource peut porter plusieurs services Argus. */
export const serviceTitle = (service: SoftwareService) =>
  service.name || service.resource_name;

/** Ordonne un groupe du plus important au moins important : mentions de
 *  sécurité et versions majeures d'abord, versions non vérifiées en tête des
 *  vérifications, puis ordre alphabétique stable. */
export function compareServices(a: SoftwareService, b: SoftwareService) {
  return (
    updateGroups.indexOf(a.group) - updateGroups.indexOf(b.group) ||
    (a.group === "review"
      ? Number(a.known) - Number(b.known) ||
        (situationRank[a.situation] ?? 9) - (situationRank[b.situation] ?? 9)
      : 0) ||
    Number(b.security_mentioned) - Number(a.security_mentioned) ||
    (a.level ? levelRank[a.level] : 3) - (b.level ? levelRank[b.level] : 3) ||
    serviceTitle(a).localeCompare(serviceTitle(b), undefined, {
      sensitivity: "base",
    }) ||
    a.id.localeCompare(b.id)
  );
}

/** Ressources portant au moins un logiciel dont une mise à jour est à appliquer.
 *  Une mise à jour disponible est une information de suivi, pas un Incident. */
export const updateTargetIds = (services: SoftwareService[]) =>
  new Set(services.filter((service) => service.group === "apply").map((service) => service.target_id));

/** Hôte d'une limite de débit signalée par le worker (« rate_limited:hôte »). */
export function rateLimitedHost(service: SoftwareService): string | undefined {
  return service.state === "retry" && service.last_error.startsWith("rate_limited:")
    ? service.last_error.slice("rate_limited:".length)
    : undefined;
}

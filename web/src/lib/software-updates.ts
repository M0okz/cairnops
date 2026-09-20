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
export type SoftwareService = {
  id: string;
  target_id: string;
  name: string;
  installed_version: string;
  target_version: string;
  observed_at: string | null;
  known: boolean;
  source: ReleaseSource;
  suggested_source: ReleaseSource | null;
  confirmed_at: string | null;
  revision: number;
  state: string;
  last_error: string;
  checked_at: string | null;
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

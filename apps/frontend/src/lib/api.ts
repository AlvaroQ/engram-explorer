export interface ApiError {
  code: string;
  message: string;
  details?: unknown;
}

export class ApiRequestError extends Error {
  constructor(
    public readonly status: number,
    public readonly api: ApiError,
  ) {
    super(`${api.code}: ${api.message}`);
    this.name = 'ApiRequestError';
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  // The SPA is always served same-origin — by the Go binary in production, and
  // via Vite's /api proxy in dev — so every /api/* fetch is same-origin: no base
  // URL, no CORS, and no proxy config needed.
  const res = await fetch(path, {
    ...init,
    headers: { Accept: 'application/json', ...(init.headers ?? {}) },
  });
  if (!res.ok) {
    let payload: { error?: ApiError } = {};
    try {
      payload = (await res.json()) as { error?: ApiError };
    } catch {
      // ignore
    }
    throw new ApiRequestError(res.status, payload.error ?? { code: 'UNKNOWN', message: res.statusText });
  }
  return (await res.json()) as T;
}

export interface GraphNode {
  id: number;
  project: string | null;
  type: string | null;
  scope: string | null;
  topicKey: string | null;
  label: string | null;
  weight: number;
  duplicateCount: number;
  createdAt: string | null;
}

export type GraphEdgeRelation = 'related' | 'scoped' | 'compatible';

export interface GraphEdge {
  source: number;
  target: number;
  relation: GraphEdgeRelation;
  confidence?: number;
  reason?: string;
}

export interface GraphMeta {
  generatedAt: string;
  totalObservations: number;
  shownNodes: number;
  duplicateGroups: number;
  edgeCounts: { related: number; scoped: number; compatible: number };
  projects: number;
  truncated: boolean;
  truncatedAt: number | null;
}

export interface GraphResponse {
  nodes: GraphNode[];
  edges: GraphEdge[];
  meta: GraphMeta;
}

export interface OverviewResponse {
  kpis: { sessions: number; observations: number; prompts: number; projects: number };
  activity_30d: Array<{ day: string; count: number }>;
  by_type: Array<{ type: string; count: number }>;
  recent_observations: Array<{
    id: number;
    type: string;
    title: string | null;
    project: string | null;
    created_at: string | null;
  }>;
  sync_summary: {
    enrolled_count: number;
    healthy_count: number;
    broken_count: number;
    pending_mutations_total: number;
  };
}

export interface HealthResponse {
  ok: boolean;
  db: { ok: boolean; path: string };
  daemon: { ok: boolean; url: string; error?: { code: string; message: string } };
  uptime_s: number;
}

export interface SyncIssue {
  code: string;
  severity: 'HIGH' | 'MEDIUM' | 'LOW' | 'INFO';
  message: string;
  project: string | null;
  hint: string | null;
  evidence?: Record<string, unknown>;
}

export interface SyncIssuesResponse {
  issues: SyncIssue[];
  generated_at: string;
}

export interface SyncProjectRow {
  project: string;
  obs_count: number;
  sessions_count: number;
  prompts_count: number;
  last_activity: string | null;
  enrolled: boolean;
  enrolled_at: string | null;
  lifecycle: string | null;
  last_enqueued_seq: number | null;
  last_acked_seq: number | null;
  last_pulled_seq: number | null;
  consecutive_failures: number | null;
  backoff_until: string | null;
  lease_owner: string | null;
  lease_until: string | null;
  last_error: string | null;
  reason_code: string | null;
  reason_message: string | null;
  pending_mutations: number;
  status: 'healthy' | 'degraded' | 'broken' | 'not_enrolled' | 'idle';
}

export interface SyncProjectsResponse {
  global_target: {
    target_key: string;
    lifecycle: string | null;
    last_enqueued_seq: number | null;
    last_acked_seq: number | null;
    pending_mutations: number;
  } | null;
  projects: SyncProjectRow[];
}

export interface ObservationRow {
  id: number;
  session_id: string | null;
  type: string;
  title: string | null;
  content: string | null;
  tool_name: string | null;
  project: string | null;
  scope: string | null;
  topic_key: string | null;
  normalized_hash: string | null;
  revision_count: number | null;
  duplicate_count: number | null;
  last_seen_at: string | null;
  created_at: string | null;
  updated_at: string | null;
  deleted_at: string | null;
  sync_id: string | null;
}

export interface ObservationListResponse {
  items: ObservationRow[];
  nextCursor: string | null;
  total: number | null;
}

export interface ObservationDetailResponse {
  observation: ObservationRow;
  revisions: ObservationRow[];
}

export interface ProjectStats {
  project: string;
  obs_count: number;
  sessions_count: number;
  prompts_count: number;
  last_activity: string | null;
  pending_mutations: number;
  sync_updated_at: string | null;
}

export type ActivityRange = '7d' | '30d' | '90d';

export interface ActivityResponse {
  range: ActivityRange;
  generated_at: string;
  rows: Array<{ day: string; project: string; count: number }>;
}

// --- Claude Code session analytics (/api/cc-sessions/stats) ---
export interface CCUsageDTO {
  input_tokens: number;
  output_tokens: number;
  cache_write_5m_tokens: number;
  cache_write_1h_tokens: number;
  cache_read_tokens: number;
  total_tokens: number;
  cost_usd: number;
  unknown_model: boolean;
}
export interface CCProjectStat {
  project: string;
  sessions: number;
  usage: CCUsageDTO;
}
export interface CCModelStat {
  model: string;
  sessions: number;
  usage: CCUsageDTO;
}
export interface CCDayStat {
  day: string;
  sessions: number;
  usage: CCUsageDTO;
}
export interface CCTopSessionStat {
  id: string;
  project_folder: string;
  project: string;
  first_prompt: string;
  started_at: string;
  usage: CCUsageDTO;
}
export interface CCStatsResponse {
  generated_at: string;
  sessions: number;
  has_unknown_model: boolean;
  totals: CCUsageDTO;
  by_project: CCProjectStat[];
  by_model: CCModelStat[];
  over_time: CCDayStat[];
  top_sessions: CCTopSessionStat[];
}

export interface ObservationListParams {
  project?: string[];
  type?: string[];
  tool_name?: string[];
  scope?: 'project' | 'personal' | 'all';
  topic_key?: string;
  q?: string;
  from?: string;
  to?: string;
  include_deleted?: boolean;
  only_deleted?: boolean;
  cursor?: string;
  limit?: number;
}

function buildSearch(params: ObservationListParams): string {
  const sp = new URLSearchParams();
  for (const project of params.project ?? []) sp.append('project', project);
  for (const type of params.type ?? []) sp.append('type', type);
  for (const tool of params.tool_name ?? []) sp.append('tool_name', tool);
  if (params.scope && params.scope !== 'all') sp.set('scope', params.scope);
  if (params.topic_key) sp.set('topic_key', params.topic_key);
  if (params.q) sp.set('q', params.q);
  if (params.from) sp.set('from', params.from);
  if (params.to) sp.set('to', params.to);
  if (params.include_deleted) sp.set('include_deleted', 'true');
  if (params.only_deleted) sp.set('only_deleted', 'true');
  if (params.cursor) sp.set('cursor', params.cursor);
  if (params.limit !== undefined) sp.set('limit', String(params.limit));
  return sp.toString();
}

export interface SessionRow {
  id: string;
  project: string | null;
  directory: string | null;
  started_at: string | null;
  ended_at: string | null;
  summary: string | null;
}

export interface SessionListItem extends SessionRow {
  obs_count: number;
  prompts_count: number;
  last_activity: string | null;
  tags?: string[];
  recent_title?: string | null;
}

export interface SessionListResponse {
  items: SessionListItem[];
  nextCursor: string | null;
}

export interface SessionTimelineEvent {
  kind: 'observation' | 'prompt';
  id: number;
  at: string | null;
  type: string | null;
  title: string | null;
  content: string | null;
  project: string | null;
  tool_name: string | null;
}

export interface SessionDetailResponse {
  session: SessionRow;
  stats: {
    obs_total: number;
    prompts_total: number;
    by_type: Array<{ type: string; count: number }>;
  };
  events: SessionTimelineEvent[];
}

export interface SessionListParams {
  project?: string[];
  from?: string;
  to?: string;
  has_summary?: boolean;
  cursor?: string;
  limit?: number;
  sort?: 'started' | 'recent_activity';
  enrich?: boolean;
}

function buildSessionSearch(p: SessionListParams): string {
  const sp = new URLSearchParams();
  for (const project of p.project ?? []) sp.append('project', project);
  if (p.from) sp.set('from', p.from);
  if (p.to) sp.set('to', p.to);
  if (p.has_summary !== undefined) sp.set('has_summary', String(p.has_summary));
  if (p.cursor) sp.set('cursor', p.cursor);
  if (p.limit !== undefined) sp.set('limit', String(p.limit));
  if (p.sort) sp.set('sort', p.sort);
  if (p.enrich) sp.set('enrich', 'true');
  return sp.toString();
}

export interface SyncProjectDetailResponse {
  summary: SyncProjectRow;
  pending: Array<{
    seq: number;
    target_key: string;
    entity: string;
    entity_key: string | null;
    op: string;
    source: string | null;
    occurred_at: string | null;
    acked_at: string | null;
    project: string | null;
  }>;
  upgrade_state: {
    project: string;
    stage: string | null;
    repair_class: string | null;
    findings: unknown;
    updated_at: string | null;
  } | null;
}

export interface PromptRow {
  id: number;
  session_id: string | null;
  content: string | null;
  project: string | null;
  created_at: string | null;
  sync_id: string | null;
}

export interface TopicRow {
  topic_key: string;
  project: string | null;
  obs_count: number;
  revisions: number;
  last_updated: string | null;
}

export interface MetaResponse {
  name: string;
  version: string;
  node: string;
}

export interface CloudCapabilities {
  enroll: boolean;
  unenroll: boolean;
  raw: string;
}

export interface ProjectOverviewResponse {
  project: string;
  kpis: {
    observations: number;
    sessions: number;
    prompts: number;
    topics: number;
    last_activity: string | null;
  };
  activity_30d: Array<{ day: string; count: number }>;
  by_type: Array<{ type: string; count: number }>;
  by_tool: Array<{ tool_name: string; count: number }>;
  top_topics: Array<{ topic_key: string; obs_count: number; last_updated: string | null }>;
  recent_observations: Array<{
    id: number;
    type: string;
    title: string | null;
    created_at: string | null;
    topic_key: string | null;
    tool_name: string | null;
  }>;
  recent_sessions: Array<{
    id: string;
    started_at: string | null;
    ended_at: string | null;
    obs_count: number;
    summary: string | null;
  }>;
  recent_prompts: Array<{ id: number; content: string | null; created_at: string | null }>;
}

export interface RenameProjectResponse {
  observations: number;
  sessions: number;
  userPrompts: number;
}

export interface ObservationUpdateResponse {
  id: number;
  mutation_seq: number | null;
  enrolled: boolean;
  updated: Partial<Pick<ObservationRow, 'type' | 'title' | 'topic_key' | 'content'>>;
}

export interface DbMergeCounts {
  observations: number;
  sessions: number;
  prompts: number;
  relations: number;
}

export interface DbImportResponse {
  backupPath: string;
  inserted: DbMergeCounts;
  skipped: DbMergeCounts;
}

export interface ObservationSearchItem extends ObservationRow {
  snippet: string;
}

export const api = {
  health: () => request<HealthResponse>('/api/health'),
  meta: () => request<MetaResponse>('/api/meta'),
  overview: () => request<OverviewResponse>('/api/overview'),
  listPrompts: (project?: string) =>
    request<{ items: PromptRow[] }>(
      project ? `/api/prompts?project=${encodeURIComponent(project)}` : `/api/prompts`,
    ),
  searchPrompts: (q: string) =>
    request<{ items: Array<PromptRow & { snippet: string }> }>(
      `/api/prompts/search?q=${encodeURIComponent(q)}`,
    ),
  searchObservations: (q: string, limit = 50) =>
    request<{ items: ObservationSearchItem[] }>(
      `/api/observations/search?q=${encodeURIComponent(q)}${limit !== 50 ? `&limit=${String(limit)}` : ''}`,
    ),
  listTopics: (project?: string) =>
    request<{ items: TopicRow[] }>(
      project ? `/api/topics?project=${encodeURIComponent(project)}` : `/api/topics`,
    ),
  syncIssues: () => request<SyncIssuesResponse>('/api/sync/issues'),
  syncProjects: () => request<SyncProjectsResponse>('/api/sync/projects'),
  syncProjectDetail: (project: string) =>
    request<SyncProjectDetailResponse>(`/api/sync/projects/${encodeURIComponent(project)}`),
  cloudCapabilities: () => request<CloudCapabilities>('/api/cloud/capabilities'),
  projectOverview: (project: string) =>
    request<ProjectOverviewResponse>(`/api/projects/${encodeURIComponent(project)}/overview`),
  listProjects: () => request<{ items: ProjectStats[] }>('/api/projects'),
  activityByProject: (range: ActivityRange) =>
    request<ActivityResponse>(`/api/activity?range=${range}`),
  ccSessionsStats: () => request<CCStatsResponse>('/api/cc-sessions/stats'),
  listObservations: (params: ObservationListParams) =>
    request<ObservationListResponse>(`/api/observations?${buildSearch(params)}`),
  getObservation: (id: number) => request<ObservationDetailResponse>(`/api/observations/${String(id)}`),
  listSessions: (params: SessionListParams) =>
    request<SessionListResponse>(`/api/sessions?${buildSessionSearch(params)}`),
  getSession: (id: string) =>
    request<SessionDetailResponse>(`/api/sessions/${encodeURIComponent(id)}`),
  enrollProject: (project: string) =>
    request<{ ok: true; project: string; output: string }>(`/api/cloud/enroll`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ project, confirm: true }),
    }),
  unenrollProject: (project: string) =>
    request<{ ok: true; project: string; output: string }>(`/api/cloud/unenroll`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ project, confirm: true }),
    }),
  cloudSyncProject: (project: string) =>
    request<CloudSyncProjectResponse>(`/api/cloud/sync`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ project }),
    }),
  cloudSyncAll: () =>
    request<CloudSyncAllResponse>(`/api/cloud/sync-all`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    }),
  listOrphans: () => request<OrphansResponse>('/api/orphans'),
  // Backend write routes are plural: /api/{observations,sessions,prompts}/...
  // The entity arg is singular, so pluralize with `+s` to hit the real route
  // (a singular path falls through to the catch-all and returns 405).
  assignProject: (entity: 'observation' | 'session' | 'prompt', id: string | number, project: string) =>
    request<AssignProjectResponse>(`/api/${entity}s/${encodeURIComponent(String(id))}/project`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ project }),
    }),
  deleteEntity: (entity: 'observation' | 'session' | 'prompt', id: string | number) =>
    request<DeleteEntityResponse>(`/api/${entity}s/${encodeURIComponent(String(id))}`, {
      method: 'DELETE',
    }),
  graph: (project?: string) =>
    request<GraphResponse>(project ? `/api/graph?project=${encodeURIComponent(project)}` : '/api/graph'),
  renameProject: (project: string, body: { target: string; mode: 'rename' | 'merge' }) =>
    request<RenameProjectResponse>(
      `/api/projects/${encodeURIComponent(project)}/rename`,
      { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) },
    ),
  updateObservation: (
    id: number,
    patch: { type?: string; title?: string | null; topic_key?: string | null; content?: string | null },
  ) =>
    request<ObservationUpdateResponse>(`/api/observations/${String(id)}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch),
    }),
  listObservationTypes: () => request<{ items: string[] }>('/api/observations/types'),
  exportDb: async (): Promise<Blob> => {
    const res = await fetch('/api/db/export', { headers: { Accept: 'application/octet-stream' } });
    if (!res.ok) {
      let payload: { error?: ApiError } = {};
      try {
        payload = (await res.json()) as { error?: ApiError };
      } catch {
        // ignore
      }
      throw new ApiRequestError(res.status, payload.error ?? { code: 'UNKNOWN', message: res.statusText });
    }
    return res.blob();
  },
  importDb: (file: File): Promise<DbImportResponse> => {
    const form = new FormData();
    form.append('file', file);
    // Note: do NOT set Content-Type — the browser adds the multipart boundary.
    return request<DbImportResponse>('/api/db/import', { method: 'POST', body: form });
  },
};

export interface OrphanObservation {
  id: number;
  type: string;
  title: string | null;
  tool_name: string | null;
  topic_key: string | null;
  created_at: string | null;
  updated_at: string | null;
  session_id: string | null;
  sync_id: string | null;
}

export interface OrphanSession {
  id: string;
  directory: string | null;
  started_at: string | null;
  ended_at: string | null;
  summary: string | null;
}

export interface OrphanPrompt {
  id: number;
  content: string | null;
  session_id: string | null;
  created_at: string | null;
  sync_id: string | null;
}

export interface OrphansResponse {
  observations: OrphanObservation[];
  sessions: OrphanSession[];
  prompts: OrphanPrompt[];
  totals: { observations: number; sessions: number; prompts: number };
}

export interface AssignProjectResponse {
  entity: 'observation' | 'session' | 'prompt';
  id: string | number;
  project: string;
  mutation_seq: number | null;
  enrolled: boolean;
}

export interface DeleteEntityResponse {
  entity: 'observation' | 'session' | 'prompt';
  id: string | number;
  mode: 'soft' | 'hard';
  mutation_seq: number | null;
  enrolled: boolean;
}

export interface CloudSyncProjectResponse {
  ok: true;
  project: string;
  action: 'sync';
  output: string;
}

export interface CloudSyncAllProjectResult {
  project: string;
  ok: boolean;
  output?: string;
  error?: { code: string; message: string; stderr?: string };
}

export interface CloudSyncAllResponse {
  total: number;
  ok: number;
  failed: number;
  results: CloudSyncAllProjectResult[];
}

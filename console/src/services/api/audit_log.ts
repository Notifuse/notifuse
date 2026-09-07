import { api } from "./client";

// The audit log, read side. Recording is the server's business (and the
// licence's); everything here works in every licence state on whatever was
// recorded.

export type AuditOutcome = "success" | "failure" | "denied";
export type AuditActorType = "user" | "api_key" | "system" | "anonymous";
export type AuditScope = "workspace" | "deployment";
export type AuditExportFormat = "csv" | "ndjson";

export interface AuditChange {
  old: unknown;
  new: unknown;
}

export interface AuditLog {
  id: string;
  occurred_at: string;
  workspace_id?: string;
  action: string;
  category: string;
  outcome: AuditOutcome;
  status_code?: number;
  actor_type: AuditActorType;
  actor_id?: string;
  actor_email?: string;
  actor_name?: string;
  actor_role?: string;
  auth_method?: string;
  target_type?: string;
  target_id?: string;
  target_name?: string;
  ip_address?: string;
  user_agent?: string;
  request_id?: string;
  changes?: Record<string, AuditChange>;
  metadata?: Record<string, unknown>;
}

export interface AuditLogListParams {
  scope?: AuditScope;
  workspace_id?: string;
  actions?: string[];
  categories?: string[];
  actor_id?: string;
  actor_email?: string;
  actor_type?: AuditActorType;
  outcome?: AuditOutcome;
  target_type?: string;
  target_id?: string;
  ip_address?: string;
  from?: string;
  to?: string;
  cursor?: string;
  limit?: number;
}

export interface AuditLogListResult {
  logs: AuditLog[];
  next_cursor?: string;
  has_more: boolean;
}

export interface AuditActionDescriptor {
  action: string;
  category: string;
  target_type?: string;
}

export interface AuditActionsResult {
  actions: AuditActionDescriptor[];
  categories: string[];
}

// Per-workspace retention. `retention_days` absent means the deployment
// default; 0 keeps forever; otherwise 30–3650.
export interface AuditLogSettings {
  retention_days?: number | null;
}

export const AUDIT_LOG_DEFAULT_RETENTION_DAYS = 365;
export const AUDIT_LOG_MIN_RETENTION_DAYS = 30;
export const AUDIT_LOG_MAX_RETENTION_DAYS = 3650;
export const AUDIT_LOG_EXPORT_MAX_ROWS = 100_000;

// Empty values are left out so the server sees a filter, not a field set to "".
export function buildAuditLogQuery(
  params: AuditLogListParams,
): URLSearchParams {
  const query = new URLSearchParams();
  const set = (key: string, value: string | number | undefined) => {
    if (value === undefined || value === "" || value === null) return;
    query.set(key, String(value));
  };
  set("scope", params.scope);
  set("workspace_id", params.workspace_id);
  if (params.actions?.length) query.set("actions", params.actions.join(","));
  if (params.categories?.length)
    query.set("categories", params.categories.join(","));
  set("actor_id", params.actor_id);
  set("actor_email", params.actor_email);
  set("actor_type", params.actor_type);
  set("outcome", params.outcome);
  set("target_type", params.target_type);
  set("target_id", params.target_id);
  set("ip_address", params.ip_address);
  set("from", params.from);
  set("to", params.to);
  set("cursor", params.cursor);
  set("limit", params.limit);
  return query;
}

export const auditLogApi = {
  list: (params: AuditLogListParams): Promise<AuditLogListResult> =>
    api.get<AuditLogListResult>(
      `/api/auditLogs.list?${buildAuditLogQuery(params).toString()}`,
    ),

  get: (
    id: string,
    scope: AuditScope,
    workspaceId?: string,
  ): Promise<{ log: AuditLog }> => {
    const query = buildAuditLogQuery({ scope, workspace_id: workspaceId });
    query.set("id", id);
    return api.get<{ log: AuditLog }>(`/api/auditLogs.get?${query.toString()}`);
  },

  actions: (): Promise<AuditActionsResult> =>
    api.get<AuditActionsResult>("/api/auditLogs.actions"),

  // A streaming download; the caller hands the blob to the browser.
  export: (
    params: AuditLogListParams,
    format: AuditExportFormat,
  ): Promise<Blob> =>
    api.download("/api/auditLogs.export", {
      ...params,
      cursor: undefined,
      format,
    }),

  setWorkspaceSettings: (
    workspaceId: string,
    settings: AuditLogSettings,
  ): Promise<void> =>
    api
      .post<{ status: string }>("/api/workspaces.setAuditLogSettings", {
        workspace_id: workspaceId,
        settings,
      })
      .then(() => undefined),
};

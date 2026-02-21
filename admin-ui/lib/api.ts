/**
 * Admin API client — wraps all /api/admin/* endpoints.
 * Next.js rewrites /api/admin/* → Gateway /admin/* (see next.config.ts).
 */

const BASE = "/api/admin";

function getHeaders(): HeadersInit {
  // Cookie-based auth: the session cookie is sent automatically.
  // For direct API access, also support ADMIN_KEY env / localStorage.
  const key =
    typeof window !== "undefined" ? localStorage.getItem("admin_key") : null;
  return key ? { Authorization: `Bearer ${key}` } : {};
}

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...getHeaders(),
      ...init?.headers,
    },
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err?.error?.message ?? err?.error ?? res.statusText);
  }
  return res.json() as Promise<T>;
}

// --- Types ---

export interface VirtualKey {
  id: string;
  key_prefix: string;
  name: string;
  is_active: boolean;
  rpm_limit?: number;
  tpm_limit?: number;
  budget_usd?: number;
  allowed_models?: string[];
  blocked_models?: string[];
  created_at: string;
  updated_at: string;
  last_used_at?: string;
}

export interface CreateKeyPayload {
  name: string;
  rpm_limit?: number;
  tpm_limit?: number;
  budget_usd?: number;
  allowed_models?: string[];
  expires_at?: string;
}

export interface ProviderKey {
  id: string;
  provider: string;
  key_alias: string;
  key_preview: string;
  is_active: boolean;
  weight: number;
  monthly_budget_usd?: number;
  current_month_spend: number;
  use_count: number;
  created_at: string;
  last_used_at?: string;
}

export interface LogEntry {
  request_id: string;
  timestamp: string;
  model: string;
  provider: string;
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  cost_usd: number;
  latency_ms: number;
  status_code: number;
  error_code?: string;
  error_message?: string;
  finish_reason?: string;
  is_streaming: boolean;
}

export interface UsageSummary {
  total_requests: number;
  total_tokens: number;
  total_cost_usd: number;
  prompt_tokens: number;
  completion_tokens: number;
  error_count: number;
  by_model: ModelBreakdown[];
}

export interface ModelBreakdown {
  model: string;
  provider: string;
  request_count: number;
  total_tokens: number;
  cost_usd: number;
}

export interface TopSpender {
  virtual_key_id: string;
  request_count: number;
  total_tokens: number;
  cost_usd: number;
}

export interface Organization {
  id: string;
  name: string;
  created_at: string;
  updated_at: string;
}

export interface Team {
  id: string;
  org_id?: string;
  name: string;
  created_at: string;
  updated_at: string;
}

// --- Virtual Keys ---

export const keys = {
  list: () => apiFetch<{ data: VirtualKey[] }>("/keys").then((r) => r.data),
  get: (id: string) => apiFetch<VirtualKey>(`/keys/${id}`),
  create: (payload: CreateKeyPayload) =>
    apiFetch<{ key: string } & VirtualKey>("/keys", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  update: (id: string, payload: Partial<CreateKeyPayload> & { is_active?: boolean }) =>
    apiFetch<VirtualKey>(`/keys/${id}`, {
      method: "PATCH",
      body: JSON.stringify(payload),
    }),
  deactivate: (id: string) =>
    fetch(`${BASE}/keys/${id}`, { method: "DELETE", headers: getHeaders() }),
  regenerate: (id: string) =>
    apiFetch<{ key: string } & VirtualKey>(`/keys/${id}/regenerate`, { method: "POST" }),
};

// --- Provider Keys ---

export const providerKeys = {
  list: (provider?: string) =>
    apiFetch<{ data: ProviderKey[] }>(
      `/provider-keys${provider ? `?provider=${provider}` : ""}`
    ).then((r) => r.data),
};

// --- Logs ---

export const logs = {
  list: (params?: { key_id?: string; limit?: number; from?: string; to?: string }) => {
    const q = new URLSearchParams();
    if (params?.key_id) q.set("key_id", params.key_id);
    if (params?.limit) q.set("limit", String(params.limit));
    if (params?.from) q.set("from", params.from);
    if (params?.to) q.set("to", params.to);
    return apiFetch<{ data: LogEntry[] }>(`/logs?${q}`).then((r) => r.data);
  },
  get: (requestId: string) => apiFetch<LogEntry>(`/logs/${requestId}`),
};

// --- Usage ---

export const usage = {
  summary: (entityType: string, entityId: string, period = "monthly") =>
    apiFetch<UsageSummary>(
      `/usage/summary?entity_type=${entityType}&entity_id=${entityId}&period=${period}`
    ),
  topSpenders: (limit = 10, period = "monthly") =>
    apiFetch<{ data: TopSpender[] }>(
      `/usage/top-spenders?limit=${limit}&period=${period}`
    ).then((r) => r.data),
};

// --- Organizations ---

export const orgs = {
  list: () => apiFetch<{ data: Organization[] }>("/organizations").then((r) => r.data),
  create: (name: string) =>
    apiFetch<Organization>("/organizations", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),
};

// --- Teams ---

export const teams = {
  list: (orgId?: string) =>
    apiFetch<{ data: Team[] }>(
      `/teams${orgId ? `?org_id=${orgId}` : ""}`
    ).then((r) => r.data),
};

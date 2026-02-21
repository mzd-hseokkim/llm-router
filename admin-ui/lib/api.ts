/**
 * Admin API client — wraps all /api/admin/* endpoints.
 * Next.js rewrites /api/admin/* → Gateway /admin/* (see next.config.ts).
 */

const BASE = "/api/admin";

// Authorization is handled by the Next.js middleware which reads the
// httpOnly admin_session cookie and injects the Authorization header.
// No client-side key storage needed.

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
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

export interface Provider {
  id: string;
  name: string;
  adapter_type: string;
  display_name: string;
  base_url?: string;
  is_enabled: boolean;
  config_json?: Record<string, unknown>;
  sort_order: number;
  model_count?: number;
  created_at: string;
  updated_at: string;
}

export interface CreateProviderPayload {
  name: string;
  adapter_type: string;
  display_name?: string;
  base_url?: string;
  is_enabled?: boolean;
  config_json?: Record<string, unknown>;
  sort_order?: number;
}

export interface UpdateProviderPayload {
  display_name?: string;
  base_url?: string;
  is_enabled?: boolean;
  config_json?: Record<string, unknown>;
  sort_order?: number;
}

export interface Model {
  id: string;
  provider_id: string;
  model_id: string;
  model_name: string;
  display_name?: string;
  is_enabled: boolean;
  input_per_million_tokens: number;
  output_per_million_tokens: number;
  context_window?: number;
  max_output_tokens?: number;
  supports_streaming: boolean;
  supports_tools: boolean;
  supports_vision: boolean;
  tags?: string[];
  sort_order: number;
  created_at: string;
  updated_at: string;
}

export interface CreateModelPayload {
  model_id: string;
  model_name: string;
  display_name?: string;
  is_enabled?: boolean;
  input_per_million_tokens?: number;
  output_per_million_tokens?: number;
  context_window?: number;
  max_output_tokens?: number;
  supports_streaming?: boolean;
  supports_tools?: boolean;
  supports_vision?: boolean;
  tags?: string[];
  sort_order?: number;
}

export type UpdateModelPayload = Partial<CreateModelPayload>;

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
    fetch(`${BASE}/keys/${id}`, { method: "DELETE" }),
  regenerate: (id: string) =>
    apiFetch<{ key: string } & VirtualKey>(`/keys/${id}/regenerate`, { method: "POST" }),
};

// --- Providers ---

export const providers = {
  list: () =>
    apiFetch<{ data: Provider[] }>("/providers").then((r) => r.data),
  get: (id: string) => apiFetch<Provider>(`/providers/${id}`),
  create: (payload: CreateProviderPayload) =>
    apiFetch<Provider>("/providers", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  update: (id: string, payload: UpdateProviderPayload) =>
    apiFetch<Provider>(`/providers/${id}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  delete: (id: string) =>
    fetch(`${BASE}/providers/${id}`, { method: "DELETE" }),
  models: {
    list: (providerId: string) =>
      apiFetch<{ data: Model[] }>(`/providers/${providerId}/models`).then((r) => r.data),
    create: (providerId: string, payload: CreateModelPayload) =>
      apiFetch<Model>(`/providers/${providerId}/models`, {
        method: "POST",
        body: JSON.stringify(payload),
      }),
    update: (providerId: string, modelId: string, payload: UpdateModelPayload) =>
      apiFetch<Model>(`/providers/${providerId}/models/${modelId}`, {
        method: "PUT",
        body: JSON.stringify(payload),
      }),
    delete: (providerId: string, modelId: string) =>
      fetch(`${BASE}/providers/${providerId}/models/${modelId}`, { method: "DELETE" }),
  },
};

// --- Provider Keys ---

export const providerKeys = {
  list: (provider?: string) =>
    apiFetch<{ data: ProviderKey[] }>(
      `/provider-keys${provider ? `?provider=${provider}` : ""}`
    ).then((r) => r.data),
  get: (id: string) => apiFetch<ProviderKey>(`/provider-keys/${id}`),
  create: (payload: {
    provider: string;
    key_alias: string;
    api_key: string;
    group_name?: string;
    tags?: string[];
    weight?: number;
    is_active?: boolean;
  }) =>
    apiFetch<ProviderKey>("/provider-keys", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  update: (id: string, payload: Partial<{
    key_alias: string;
    group_name: string;
    tags: string[];
    weight: number;
    is_active: boolean;
    monthly_budget_usd: number;
  }>) =>
    apiFetch<ProviderKey>(`/provider-keys/${id}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  delete: (id: string) =>
    fetch(`${BASE}/provider-keys/${id}`, { method: "DELETE" }),
  rotate: (id: string, newApiKey: string) =>
    apiFetch<ProviderKey>(`/provider-keys/${id}/rotate`, {
      method: "PUT",
      body: JSON.stringify({ new_api_key: newApiKey }),
    }),
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

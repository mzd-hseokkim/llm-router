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
  api_key?: string; // optional: creates default key atomically on the server
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

export interface User {
  id: string;
  org_id?: string;
  team_id?: string;
  email: string;
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
  list: () => apiFetch<{ data: Organization[] }>("/organizations").then((r) => r.data ?? []),
  create: (name: string) =>
    apiFetch<Organization>("/organizations", { method: "POST", body: JSON.stringify({ name }) }),
  update: (id: string, name: string) =>
    apiFetch<Organization>(`/organizations/${id}`, { method: "PUT", body: JSON.stringify({ name }) }),
};

// --- Teams ---

export const teams = {
  list: (orgId?: string) =>
    apiFetch<{ data: Team[] }>(`/teams${orgId ? `?org_id=${orgId}` : ""}`).then((r) => r.data ?? []),
  create: (orgId: string, name: string) =>
    apiFetch<Team>("/teams", { method: "POST", body: JSON.stringify({ org_id: orgId, name }) }),
  update: (id: string, name: string) =>
    apiFetch<Team>(`/teams/${id}`, { method: "PUT", body: JSON.stringify({ name }) }),
};

// --- Users ---

export const users = {
  list: (orgId?: string) =>
    apiFetch<{ data: User[] }>(`/users${orgId ? `?org_id=${orgId}` : ""}`).then((r) => r.data ?? []),
  create: (orgId: string, email: string, teamId?: string) =>
    apiFetch<User>("/users", {
      method: "POST",
      body: JSON.stringify({ org_id: orgId, email, team_id: teamId }),
    }),
  update: (id: string, email: string, teamId?: string) =>
    apiFetch<User>(`/users/${id}`, {
      method: "PUT",
      body: JSON.stringify({ email, team_id: teamId ?? null }),
    }),
};

// --- Guardrails ---

export interface GuardrailPolicy {
  id: string;
  guardrail_type: string;
  is_enabled: boolean;
  action: string;
  engine?: string;
  config_json: Record<string, unknown>;
  sort_order: number;
  created_at: string;
  updated_at: string;
}

export interface UpdateGuardrailPayload {
  is_enabled?: boolean;
  action?: string;
  engine?: string;
  config_json?: Record<string, unknown>;
  sort_order?: number;
}

export const guardrails = {
  list: () =>
    apiFetch<{ data: GuardrailPolicy[] }>("/guardrails").then((r) => r.data),
  get: (type: string) => apiFetch<GuardrailPolicy>(`/guardrails/${type}`),
  update: (type: string, payload: UpdateGuardrailPayload) =>
    apiFetch<GuardrailPolicy>(`/guardrails/${type}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  updateAll: (policies: Array<{ guardrail_type: string } & UpdateGuardrailPayload>) =>
    apiFetch<{ data: GuardrailPolicy[] }>("/guardrails", {
      method: "PUT",
      body: JSON.stringify({ policies }),
    }).then((r) => r.data),
};

// --- Budgets ---

export interface Budget {
  entity_type: string;
  entity_id: string;
  period: string;
  soft_limit_usd: number;
  hard_limit_usd: number;
  period_start: string;
  period_end: string;
}

export interface CreateBudgetPayload {
  entity_type: string;
  entity_id: string;
  period: string;
  soft_limit_usd?: number;
  hard_limit_usd?: number;
}

export const budgets = {
  list: (entityType: string, entityId: string) =>
    apiFetch<Budget[]>(`/budgets/${entityType}/${entityId}`),
  create: (payload: CreateBudgetPayload) =>
    apiFetch<Budget>("/budgets", { method: "POST", body: JSON.stringify(payload) }),
  reset: (id: string) =>
    apiFetch<{ error: string }>(`/budgets/${id}/reset`, { method: "POST" }),
};

// --- A/B Tests ---

export interface ABTestTrafficSplit {
  variant: string;
  weight: number;
}

export interface ABTestTarget {
  model: string;
  sample_rate?: number;
}

export interface ABTest {
  id: string;
  name: string;
  status: string;
  traffic_split: ABTestTrafficSplit[];
  target: ABTestTarget;
  success_metrics?: string[];
  min_samples?: number;
  confidence_level?: number;
  start_at?: string;
  end_at?: string;
}

export interface CreateABTestPayload {
  name: string;
  traffic_split: ABTestTrafficSplit[];
  target: ABTestTarget;
  success_metrics?: string[];
  min_samples?: number;
  confidence_level?: number;
  start_at?: string;
  end_at?: string;
}

export const abTests = {
  list: () => apiFetch<{ data: ABTest[] }>("/ab-tests").then((r) => r.data),
  get: (id: string) => apiFetch<ABTest>(`/ab-tests/${id}`),
  create: (payload: CreateABTestPayload) =>
    apiFetch<ABTest>("/ab-tests", { method: "POST", body: JSON.stringify(payload) }),
  results: (id: string) => apiFetch<Record<string, unknown>>(`/ab-tests/${id}/results`),
  start: (id: string) =>
    apiFetch<{ status: string }>(`/ab-tests/${id}/start`, { method: "POST" }),
  pause: (id: string) =>
    apiFetch<{ status: string }>(`/ab-tests/${id}/pause`, { method: "POST" }),
  stop: (id: string) =>
    apiFetch<{ status: string }>(`/ab-tests/${id}/stop`, { method: "POST" }),
  promote: (id: string, winner: string) =>
    apiFetch<{ status: string; winner: string }>(`/ab-tests/${id}/promote`, {
      method: "POST",
      body: JSON.stringify({ winner }),
    }),
};

// --- Prompts ---

export interface Prompt {
  id: string;
  slug: string;
  name: string;
  template: string;
  created_at: string;
  updated_at: string;
}

export interface PromptVersion {
  id: string;
  prompt_id?: string;
  version: string;
  template: string;
  is_active?: boolean;
  created_at: string;
}

export interface CreatePromptPayload {
  slug: string;
  name: string;
  template: string;
  team_id?: string;
  tags?: string[];
}

export const prompts = {
  list: (teamId?: string) =>
    apiFetch<{ data: Prompt[] }>(`/prompts${teamId ? `?team_id=${teamId}` : ""}`).then(
      (r) => r.data
    ),
  get: (slug: string) =>
    apiFetch<{ prompt: Prompt; version: PromptVersion }>(`/prompts/${slug}`),
  create: (payload: CreatePromptPayload) =>
    apiFetch<{ prompt: Prompt; version: PromptVersion }>("/prompts", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  rollback: (slug: string, version: string) =>
    apiFetch<{ message: string }>(`/prompts/${slug}/rollback/${version}`, { method: "POST" }),
  render: (slug: string, variables: Record<string, string>) =>
    apiFetch<{ rendered: string; token_count: number }>(`/prompts/${slug}/render`, {
      method: "POST",
      body: JSON.stringify({ variables }),
    }),
  diff: (slug: string, from: string, to: string) =>
    apiFetch<{ from: { version: string; template: string }; to: { version: string; template: string } }>(
      `/prompts/${slug}/diff?from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`
    ),
  versions: {
    list: (slug: string) =>
      apiFetch<{ data: PromptVersion[] }>(`/prompts/${slug}/versions`).then((r) => r.data),
    get: (slug: string, version: string) =>
      apiFetch<PromptVersion>(`/prompts/${slug}/versions/${version}`),
    create: (slug: string, payload: { version: string; template: string }) =>
      apiFetch<PromptVersion>(`/prompts/${slug}/versions`, {
        method: "POST",
        body: JSON.stringify(payload),
      }),
  },
};

// --- Routing ---

export interface RoutingConfig {
  default_strategy: string;
  providers: unknown[];
  rules: unknown[];
}

export const routing = {
  get: () => apiFetch<RoutingConfig>("/routing"),
  update: (payload: RoutingConfig) =>
    apiFetch<{ status: string }>("/routing", { method: "PUT", body: JSON.stringify(payload) }),
  reload: () => apiFetch<{ status: string }>("/routing/reload", { method: "POST" }),
};

// --- Routing Rules ---

export interface RoutingRuleMatch {
  model?: string;
  model_prefix?: string;
  model_regex?: string;
  key_id?: string;
  user_id?: string;
  team_id?: string;
  org_id?: string;
  metadata?: Record<string, string>;
  min_context_tokens?: number;
  max_context_tokens?: number;
  has_tools?: boolean;
}

export interface RoutingRuleTarget {
  provider: string;
  model: string;
  weight?: number;
}

export interface RoutingRule {
  id: string;
  name: string;
  priority: number;
  enabled: boolean;
  match: RoutingRuleMatch;
  strategy: string;
  targets: RoutingRuleTarget[];
  created_at: string;
  updated_at: string;
}

export interface CreateRoutingRulePayload {
  name: string;
  priority?: number;
  enabled?: boolean;
  match?: RoutingRuleMatch;
  strategy: string;
  targets: RoutingRuleTarget[];
}

export interface DryRunPayload {
  model: string;
  metadata?: Record<string, string>;
  messages?: Array<{ role: string; content: string }>;
}

export const routingRules = {
  list: () => apiFetch<{ data: RoutingRule[] }>("/routing/rules").then((r) => r.data),
  create: (payload: CreateRoutingRulePayload) =>
    apiFetch<RoutingRule>("/routing/rules", { method: "POST", body: JSON.stringify(payload) }),
  update: (id: string, payload: CreateRoutingRulePayload) =>
    apiFetch<RoutingRule>(`/routing/rules/${id}`, { method: "PUT", body: JSON.stringify(payload) }),
  delete: (id: string) =>
    fetch(`${BASE}/routing/rules/${id}`, { method: "DELETE" }),
  reload: () => apiFetch<{ status: string }>("/routing/rules/reload", { method: "POST" }),
  dryRun: (payload: DryRunPayload) =>
    apiFetch<{ matched_rule: string | null; strategy: string; targets: RoutingRuleTarget[]; message?: string }>(
      "/routing/test",
      { method: "POST", body: JSON.stringify(payload) }
    ),
};

// --- Audit Logs ---

export interface AuditEvent {
  timestamp: string;
  event_type: string;
  action: string;
  actor_type: string;
  actor_email: string;
  ip_address: string;
  resource_type: string;
  resource_name: string;
  request_id: string;
}

export interface AuditLogParams {
  actor_id?: string;
  event_type?: string;
  resource_id?: string;
  from?: string;
  to?: string;
  limit?: number;
  page?: number;
}

export const auditLogs = {
  list: (params?: AuditLogParams) => {
    const q = new URLSearchParams();
    if (params?.actor_id) q.set("actor_id", params.actor_id);
    if (params?.event_type) q.set("event_type", params.event_type);
    if (params?.resource_id) q.set("resource_id", params.resource_id);
    if (params?.from) q.set("from", params.from);
    if (params?.to) q.set("to", params.to);
    if (params?.limit) q.set("limit", String(params.limit));
    if (params?.page) q.set("page", String(params.page));
    return apiFetch<{ total: number; page: number; limit: number; events: AuditEvent[] }>(
      `/audit-logs?${q}`
    );
  },
  securityEvents: (params?: AuditLogParams) => {
    const q = new URLSearchParams();
    if (params?.from) q.set("from", params.from);
    if (params?.to) q.set("to", params.to);
    if (params?.limit) q.set("limit", String(params.limit));
    if (params?.page) q.set("page", String(params.page));
    return apiFetch<{ total: number; page: number; limit: number; events: AuditEvent[] }>(
      `/audit-logs/security-events?${q}`
    );
  },
};

// --- Alerts ---

export interface AlertHistoryEntry {
  id: string;
  event_type: string;
  severity: string;
  channel: string;
  status: string;
  payload: Record<string, unknown>;
  error?: string;
  sent_at: string;
}

export const alerts = {
  test: (channel?: string) =>
    apiFetch<{ status: string }>(`/alerts/test${channel ? `?channel=${channel}` : ""}`, {
      method: "POST",
    }),
  history: (limit = 100) =>
    apiFetch<{ history: AlertHistoryEntry[] }>(`/alerts/history?limit=${limit}`).then(
      (r) => r.history
    ),
};

// --- Reports ---

export interface ChargebackBreakdown {
  team_id: string;
  team_name: string;
  cost: number;
}

export interface ChargebackReport {
  period: string;
  total_cost: number;
  breakdown: ChargebackBreakdown[];
}

export interface ShowbackReport {
  team_id: string;
  period: string;
  total_cost: number;
  breakdown: unknown[];
}

export const reports = {
  chargeback: (params?: { period?: string }) =>
    apiFetch<ChargebackReport>(
      `/reports/chargeback${params?.period ? `?period=${params.period}` : ""}`
    ),
  showback: (teamId: string, period?: string) => {
    const q = new URLSearchParams({ team_id: teamId });
    if (period) q.set("period", period);
    return apiFetch<ShowbackReport>(`/reports/showback?${q}`);
  },
};

// --- MCP ---

export interface MCPServer {
  name: string;
  type: string;
  url?: string;
  status: string;
}

export interface MCPTool {
  name: string;
  description: string;
  inputSchema: Record<string, unknown>;
}

export interface CreateMCPServerPayload {
  name: string;
  type: string;
  command?: string;
  url?: string;
}

export const mcp = {
  servers: {
    list: () => apiFetch<{ servers: MCPServer[] }>("/mcp/servers").then((r) => r.servers),
    get: (name: string) => apiFetch<MCPServer>(`/mcp/servers/${name}`),
    create: (payload: CreateMCPServerPayload) =>
      apiFetch<{ name: string; message: string }>("/mcp/servers", {
        method: "POST",
        body: JSON.stringify(payload),
      }),
    delete: (name: string) =>
      fetch(`${BASE}/mcp/servers/${name}`, { method: "DELETE" }),
    health: (name: string) =>
      apiFetch<{ name: string; status: string; error?: string }>(`/mcp/servers/${name}/health`),
    tools: (name: string) =>
      apiFetch<{ tools: MCPTool[] }>(`/mcp/servers/${name}/tools`).then((r) => r.tools),
  },
  policies: {
    set: (payload: { key_id: string; tools: Array<{ name: string; allowed: boolean }> }) =>
      apiFetch<{ message: string; policy: Record<string, unknown> }>("/mcp/policies", {
        method: "POST",
        body: JSON.stringify(payload),
      }),
  },
};

// --- Circuit Breakers ---

export interface CircuitBreaker {
  provider: string;
  state: string;
  failure_count: number;
  last_failure?: string;
  reset_time?: string;
}

export const circuitBreakers = {
  list: () =>
    apiFetch<{ circuit_breakers: CircuitBreaker[] }>("/circuit-breakers").then(
      (r) => r.circuit_breakers
    ),
  reset: (provider: string) =>
    apiFetch<{ provider: string; state: string; message: string }>(
      `/circuit-breakers/${provider}/reset`,
      { method: "POST" }
    ),
};

// --- Rate Limits ---

export interface RateLimitConfig {
  key_id: string;
  rpm_limit: number;
  tpm_limit: number;
  rpm_window: string;
}

export const rateLimits = {
  get: (id: string) => apiFetch<RateLimitConfig>(`/rate-limits/${id}`),
  reset: (id: string) =>
    apiFetch<{ key_id: string; rpm_key: string; tpm_key: string; note: string; reset_at: string }>(
      `/rate-limits/${id}/reset`,
      { method: "POST" }
    ),
};

// --- Data Residency ---

export interface ResidencyPolicy {
  name: string;
  allowed_providers: string[];
  blocked_providers: string[];
  allowed_regions: string[];
}

export const residency = {
  policies: {
    list: () =>
      apiFetch<{ policies: ResidencyPolicy[] }>("/data-residency/policies").then(
        (r) => r.policies
      ),
    get: (name: string) => apiFetch<ResidencyPolicy>(`/data-residency/policies/${name}`),
  },
  validate: (payload: { policy: string; provider: string; model?: string }) =>
    apiFetch<{ compliant: boolean; reason?: string }>("/data-residency/validate", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  report: () =>
    apiFetch<{ report: ResidencyPolicy[]; policy_count: number }>("/data-residency/report"),
};

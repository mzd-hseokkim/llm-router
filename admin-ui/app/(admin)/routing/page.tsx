"use client";

import React, { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  routingRules,
  RoutingRule,
  RoutingRuleTarget,
  CreateRoutingRulePayload,
} from "@/lib/api";

// ---------------------------------------------------------------------------
// Local types
// ---------------------------------------------------------------------------

interface TargetFormRow {
  provider: string;
  model: string;
  weight: number;
}

interface MetadataRow {
  key: string;
  value: string;
}

interface RuleFormState {
  name: string;
  priority: number;
  enabled: boolean;
  strategy: string;
  targets: TargetFormRow[];
  matchModel: string;
  matchModelPrefix: string;
  matchModelRegex: string;
  matchMinTokens: string;
  matchMaxTokens: string;
  matchHasTools: boolean;
  matchMetadata: MetadataRow[];
  matchKeyId: string;
  matchUserId: string;
  matchTeamId: string;
  matchOrgId: string;
}

const UUID_REGEX =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

function isValidUuid(v: string) {
  return v === "" || UUID_REGEX.test(v);
}

const EMPTY_FORM: RuleFormState = {
  name: "",
  priority: 100,
  enabled: true,
  strategy: "direct",
  targets: [{ provider: "", model: "", weight: 0 }],
  matchModel: "",
  matchModelPrefix: "",
  matchModelRegex: "",
  matchMinTokens: "",
  matchMaxTokens: "",
  matchHasTools: false,
  matchMetadata: [],
  matchKeyId: "",
  matchUserId: "",
  matchTeamId: "",
  matchOrgId: "",
};

// ---------------------------------------------------------------------------
// StrategyBadge
// ---------------------------------------------------------------------------

const STRATEGY_COLORS: Record<string, string> = {
  direct:        "bg-blue-100 text-blue-700",
  weighted:      "bg-purple-100 text-purple-700",
  least_cost:    "bg-green-100 text-green-700",
  failover:      "bg-orange-100 text-orange-700",
  quality_first: "bg-pink-100 text-pink-700",
};

function StrategyBadge({ strategy }: { strategy: string }) {
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium
                  ${STRATEGY_COLORS[strategy] ?? "bg-slate-100 text-slate-600"}`}
    >
      {strategy}
    </span>
  );
}

// ---------------------------------------------------------------------------
// EnabledBadge
// ---------------------------------------------------------------------------

function EnabledBadge({ enabled }: { enabled: boolean }) {
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium
                  ${enabled ? "bg-green-100 text-green-700" : "bg-slate-100 text-slate-500"}`}
    >
      {enabled ? "Enabled" : "Disabled"}
    </span>
  );
}

// ---------------------------------------------------------------------------
// MatchSummary
// ---------------------------------------------------------------------------

function MatchSummary({ match }: { match: RoutingRule["match"] }) {
  const parts: string[] = [];
  if (match.model)         parts.push(`model: ${match.model}`);
  if (match.model_prefix)  parts.push(`prefix: ${match.model_prefix}`);
  if (match.model_regex)   parts.push(`regex: ${match.model_regex}`);
  if (match.max_context_tokens) parts.push(`context ≤ ${Math.round(match.max_context_tokens / 1000)}k`);
  if (match.min_context_tokens) parts.push(`context ≥ ${Math.round(match.min_context_tokens / 1000)}k`);
  if (match.has_tools)     parts.push("has_tools");
  if (match.key_id)        parts.push(`key: ${match.key_id.slice(0, 8)}…`);
  if (match.user_id)       parts.push(`user: ${match.user_id.slice(0, 8)}…`);
  if (match.team_id)       parts.push(`team: ${match.team_id.slice(0, 8)}…`);
  if (match.org_id)        parts.push(`org: ${match.org_id.slice(0, 8)}…`);
  if (match.metadata && Object.keys(match.metadata).length > 0)
    parts.push(`meta: ${Object.keys(match.metadata).join(",")}`);

  if (parts.length === 0) return <span className="text-slate-400 text-xs">Any</span>;

  return (
    <span className="text-xs text-slate-600 font-mono">
      {parts[0]}
      {parts.length > 1 && (
        <span className="text-slate-400 ml-1">+{parts.length - 1} more</span>
      )}
    </span>
  );
}

// ---------------------------------------------------------------------------
// DeleteConfirmDialog
// ---------------------------------------------------------------------------

function DeleteConfirmDialog({
  rule,
  onClose,
  onDeleted,
}: {
  rule: RoutingRule;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const qc = useQueryClient();
  const [error, setError] = useState("");

  const mutation = useMutation({
    mutationFn: async () => {
      const res = await routingRules.delete(rule.id);
      if (res.status >= 400 && res.status !== 404) {
        throw new Error(`Delete failed: ${res.status}`);
      }
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["routing-rules"] });
      onDeleted();
    },
    onError: (e: Error) => setError(e.message),
  });

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-sm p-6 space-y-4">
        <h2 className="text-lg font-semibold text-slate-900">Delete Rule</h2>
        <p className="text-sm text-slate-600">
          Are you sure you want to delete{" "}
          <strong>&ldquo;{rule.name}&rdquo;</strong>? This cannot be undone.
        </p>
        {error && (
          <p className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</p>
        )}
        <div className="flex justify-end gap-2 pt-2">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg"
          >
            Cancel
          </button>
          <button
            disabled={mutation.isPending}
            onClick={() => mutation.mutate()}
            className="px-4 py-2 text-sm bg-red-600 text-white rounded-lg disabled:opacity-50 hover:bg-red-700"
          >
            {mutation.isPending ? "Deleting…" : "Delete"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// DryRunPanel
// ---------------------------------------------------------------------------

function DryRunPanel({ rules }: { rules: RoutingRule[] }) {
  const [dryRunModel, setDryRunModel] = useState("");
  const [dryRunMetadata, setDryRunMetadata] = useState<MetadataRow[]>([]);
  const [dryRunResult, setDryRunResult] = useState<{
    matched_rule: string | null;
    strategy: string;
    targets: RoutingRuleTarget[];
    message?: string;
  } | null>(null);
  const [error, setError] = useState("");

  const mutation = useMutation({
    mutationFn: () => {
      const meta: Record<string, string> = {};
      dryRunMetadata.forEach(({ key, value }) => {
        if (key) meta[key] = value;
      });
      return routingRules.dryRun({
        model: dryRunModel,
        ...(Object.keys(meta).length > 0 ? { metadata: meta } : {}),
      });
    },
    onSuccess: (data) => {
      setDryRunResult(data);
      setError("");
    },
    onError: (e: Error) => {
      setError(e.message);
      setDryRunResult(null);
    },
  });

  function getMatchedName(matched_rule: string | null): string | null {
    if (!matched_rule) return null;
    // If it looks like a UUID, look up by id
    if (UUID_REGEX.test(matched_rule)) {
      const found = rules.find((r) => r.id === matched_rule);
      return found ? found.name : matched_rule;
    }
    return matched_rule;
  }

  return (
    <div className="bg-white rounded-xl border border-slate-200 shadow-sm p-5 space-y-4">
      <h2 className="text-base font-semibold text-slate-800">DryRun Test</h2>
      <p className="text-xs text-slate-500">
        라우팅 규칙을 실제로 적용하지 않고 어떤 규칙이 매칭되는지 테스트합니다.
      </p>

      <div className="space-y-3">
        <label className="block">
          <span className="text-xs font-medium text-slate-600">Model *</span>
          <input
            className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
            placeholder="e.g. gpt-4o"
            value={dryRunModel}
            onChange={(e) => setDryRunModel(e.target.value)}
          />
        </label>

        <div>
          <p className="text-xs font-medium text-slate-600 mb-1">
            Metadata (optional)
          </p>
          <div className="space-y-1">
            {dryRunMetadata.map((row, idx) => (
              <div key={idx} className="flex gap-2 items-center">
                <input
                  className="flex-1 border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                  placeholder="key"
                  value={row.key}
                  onChange={(e) =>
                    setDryRunMetadata((prev) =>
                      prev.map((r, i) =>
                        i === idx ? { ...r, key: e.target.value } : r
                      )
                    )
                  }
                />
                <input
                  className="flex-1 border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                  placeholder="value"
                  value={row.value}
                  onChange={(e) =>
                    setDryRunMetadata((prev) =>
                      prev.map((r, i) =>
                        i === idx ? { ...r, value: e.target.value } : r
                      )
                    )
                  }
                />
                <button
                  onClick={() =>
                    setDryRunMetadata((prev) => prev.filter((_, i) => i !== idx))
                  }
                  className="text-slate-400 hover:text-red-500 text-sm px-1"
                >
                  ×
                </button>
              </div>
            ))}
          </div>
          <button
            onClick={() =>
              setDryRunMetadata((prev) => [...prev, { key: "", value: "" }])
            }
            className="mt-1 text-xs text-brand-600 hover:underline"
          >
            + Add Metadata
          </button>
        </div>
      </div>

      <button
        disabled={!dryRunModel.trim() || mutation.isPending}
        onClick={() => mutation.mutate()}
        className="px-4 py-2 text-sm bg-slate-900 text-white rounded-lg disabled:opacity-50 hover:bg-slate-700"
      >
        {mutation.isPending ? "Testing…" : "Test"}
      </button>

      {error && (
        <p className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</p>
      )}

      {dryRunResult && (
        <div className="mt-3 rounded-lg border border-slate-200 bg-slate-50 p-4 space-y-3">
          {dryRunResult.matched_rule ? (
            <>
              <div className="flex items-center gap-2">
                <span className="text-xs font-medium text-slate-500">Matched Rule:</span>
                <span className="text-sm font-semibold text-slate-900">
                  {getMatchedName(dryRunResult.matched_rule)}
                </span>
              </div>
              <div className="flex items-center gap-2">
                <span className="text-xs font-medium text-slate-500">Strategy:</span>
                <StrategyBadge strategy={dryRunResult.strategy} />
              </div>
              {dryRunResult.targets.length > 0 ? (
                <div>
                  <span className="text-xs font-medium text-slate-500">Targets:</span>
                  <ul className="mt-1 space-y-0.5">
                    {dryRunResult.targets.map((t, i) => (
                      <li key={i} className="text-xs text-slate-700 font-mono">
                        {t.provider}/{t.model}
                        {t.weight != null ? ` (${t.weight}%)` : ""}
                      </li>
                    ))}
                  </ul>
                </div>
              ) : (
                <p className="text-xs text-slate-400">No targets available</p>
              )}
            </>
          ) : (
            <p className="text-sm text-slate-500">
              No matching rule — default routing applied
            </p>
          )}
          {dryRunResult.message && (
            <p className="text-xs text-slate-400 italic">{dryRunResult.message}</p>
          )}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// RuleDialog
// ---------------------------------------------------------------------------

function ruleToForm(rule: RoutingRule): RuleFormState {
  const m = rule.match;
  return {
    name: rule.name,
    priority: rule.priority,
    enabled: rule.enabled,
    strategy: rule.strategy,
    targets: rule.targets.map((t) => ({
      provider: t.provider,
      model: t.model,
      weight: t.weight ?? 0,
    })),
    matchModel: m.model ?? "",
    matchModelPrefix: m.model_prefix ?? "",
    matchModelRegex: m.model_regex ?? "",
    matchMinTokens: m.min_context_tokens != null ? String(m.min_context_tokens) : "",
    matchMaxTokens: m.max_context_tokens != null ? String(m.max_context_tokens) : "",
    matchHasTools: m.has_tools ?? false,
    matchMetadata: m.metadata
      ? Object.entries(m.metadata).map(([key, value]) => ({ key, value }))
      : [],
    matchKeyId: m.key_id ?? "",
    matchUserId: m.user_id ?? "",
    matchTeamId: m.team_id ?? "",
    matchOrgId: m.org_id ?? "",
  };
}

function formToPayload(form: RuleFormState): CreateRoutingRulePayload {
  const meta: Record<string, string> = {};
  form.matchMetadata.forEach(({ key, value }) => {
    if (key) meta[key] = value;
  });

  const match: CreateRoutingRulePayload["match"] = {};
  if (form.matchModel)       match.model = form.matchModel;
  if (form.matchModelPrefix) match.model_prefix = form.matchModelPrefix;
  if (form.matchModelRegex)  match.model_regex = form.matchModelRegex;
  if (form.matchMinTokens)   match.min_context_tokens = Number(form.matchMinTokens);
  if (form.matchMaxTokens)   match.max_context_tokens = Number(form.matchMaxTokens);
  if (form.matchHasTools)    match.has_tools = true;
  if (Object.keys(meta).length > 0) match.metadata = meta;
  if (form.matchKeyId)       match.key_id = form.matchKeyId;
  if (form.matchUserId)      match.user_id = form.matchUserId;
  if (form.matchTeamId)      match.team_id = form.matchTeamId;
  if (form.matchOrgId)       match.org_id = form.matchOrgId;

  return {
    name: form.name.trim(),
    priority: form.priority,
    enabled: form.enabled,
    match,
    strategy: form.strategy,
    targets: form.targets.map((t) => ({
      provider: t.provider,
      model: t.model,
      ...(form.strategy === "weighted" ? { weight: t.weight } : {}),
    })),
  };
}

function RuleDialog({
  editingRule,
  onClose,
  onSaved,
}: {
  editingRule: RoutingRule | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const qc = useQueryClient();
  const [form, setForm] = useState<RuleFormState>(
    editingRule ? ruleToForm(editingRule) : { ...EMPTY_FORM }
  );
  const [showAdvanced, setShowAdvanced] = useState(false);
  const [error, setError] = useState("");

  const isWeighted = form.strategy === "weighted";
  const isDirect = form.strategy === "direct";

  const totalWeight = isWeighted
    ? form.targets.reduce((sum, t) => sum + (Number(t.weight) || 0), 0)
    : 100;
  const weightValid = !isWeighted || totalWeight === 100;

  const minTargets = isDirect ? 1 : 2;
  const uuidFields = [form.matchKeyId, form.matchUserId, form.matchTeamId, form.matchOrgId];
  const uuidsValid = uuidFields.every(isValidUuid);

  const mutation = useMutation({
    mutationFn: () => {
      const payload = formToPayload(form);
      return editingRule
        ? routingRules.update(editingRule.id, payload)
        : routingRules.create(payload);
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["routing-rules"] });
      onSaved();
    },
    onError: (e: Error) => setError(e.message),
  });

  const canSubmit =
    form.name.trim().length > 0 &&
    form.targets.length >= minTargets &&
    form.targets.every((t) => t.provider.trim() && t.model.trim()) &&
    weightValid &&
    uuidsValid &&
    !mutation.isPending;

  function updateTarget(idx: number, field: keyof TargetFormRow, value: string | number) {
    setForm((f) => ({
      ...f,
      targets: f.targets.map((t, i) => (i === idx ? { ...t, [field]: value } : t)),
    }));
  }

  function addTarget() {
    setForm((f) => ({
      ...f,
      targets: [...f.targets, { provider: "", model: "", weight: 0 }],
    }));
  }

  function removeTarget(idx: number) {
    setForm((f) => ({
      ...f,
      targets: f.targets.filter((_, i) => i !== idx),
    }));
  }

  function onStrategyChange(strategy: string) {
    setForm((f) => {
      let targets = f.targets;
      if (strategy === "direct" && targets.length > 1) {
        targets = [targets[0]];
      } else if (strategy !== "direct" && targets.length < 2) {
        targets = [...targets, { provider: "", model: "", weight: 0 }];
      }
      return { ...f, strategy, targets };
    });
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-lg p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <h2 className="text-lg font-semibold text-slate-900">
          {editingRule ? "Edit Rule" : "Add Rule"}
        </h2>

        {error && (
          <p className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</p>
        )}

        {/* Basic Fields */}
        <div className="space-y-3">
          <label className="block">
            <span className="text-xs font-medium text-slate-600">Name *</span>
            <input
              name="name"
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              placeholder="Rule name"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            />
          </label>

          <div className="grid grid-cols-2 gap-3">
            <label className="block">
              <span className="text-xs font-medium text-slate-600">Priority</span>
              <input
                type="number"
                className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
                value={form.priority}
                onChange={(e) =>
                  setForm((f) => ({ ...f, priority: Number(e.target.value) }))
                }
              />
            </label>
            <div className="block">
              <span className="text-xs font-medium text-slate-600 block mb-2">Enabled</span>
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  className="w-4 h-4"
                  checked={form.enabled}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, enabled: e.target.checked }))
                  }
                />
                <span className="text-sm text-slate-600">Active</span>
              </label>
            </div>
          </div>
        </div>

        {/* Advanced Conditions */}
        <div className="border border-slate-200 rounded-lg overflow-hidden">
          <button
            type="button"
            onClick={() => setShowAdvanced((v) => !v)}
            className="w-full flex items-center justify-between px-4 py-3 bg-slate-50 text-sm font-medium text-slate-700 hover:bg-slate-100"
          >
            <span>Advanced Conditions</span>
            <span className="text-slate-400 text-xs">{showAdvanced ? "▲ Hide" : "▼ Show"}</span>
          </button>
          {showAdvanced && (
            <div className="p-4 space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <label className="block">
                  <span className="text-xs font-medium text-slate-600">Model (exact)</span>
                  <input
                    className="mt-1 block w-full border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                    placeholder="gpt-4o"
                    value={form.matchModel}
                    onChange={(e) =>
                      setForm((f) => ({ ...f, matchModel: e.target.value }))
                    }
                  />
                </label>
                <label className="block">
                  <span className="text-xs font-medium text-slate-600">Model Prefix</span>
                  <input
                    className="mt-1 block w-full border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                    placeholder="gpt-4"
                    value={form.matchModelPrefix}
                    onChange={(e) =>
                      setForm((f) => ({ ...f, matchModelPrefix: e.target.value }))
                    }
                  />
                </label>
              </div>
              <label className="block">
                <span className="text-xs font-medium text-slate-600">Model Regex</span>
                <input
                  className="mt-1 block w-full border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                  placeholder="^gpt-4.*"
                  value={form.matchModelRegex}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, matchModelRegex: e.target.value }))
                  }
                />
              </label>
              <div className="grid grid-cols-2 gap-3">
                <label className="block">
                  <span className="text-xs font-medium text-slate-600">Min Tokens</span>
                  <input
                    type="number"
                    className="mt-1 block w-full border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                    placeholder="0"
                    value={form.matchMinTokens}
                    onChange={(e) =>
                      setForm((f) => ({ ...f, matchMinTokens: e.target.value }))
                    }
                  />
                </label>
                <label className="block">
                  <span className="text-xs font-medium text-slate-600">Max Tokens</span>
                  <input
                    type="number"
                    className="mt-1 block w-full border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                    placeholder="0"
                    value={form.matchMaxTokens}
                    onChange={(e) =>
                      setForm((f) => ({ ...f, matchMaxTokens: e.target.value }))
                    }
                  />
                </label>
              </div>
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  className="w-4 h-4"
                  checked={form.matchHasTools}
                  onChange={(e) =>
                    setForm((f) => ({ ...f, matchHasTools: e.target.checked }))
                  }
                />
                <span className="text-sm text-slate-600">Has Tools</span>
              </label>
              {/* Metadata */}
              <div>
                <p className="text-xs font-medium text-slate-600 mb-1">Metadata</p>
                <div className="space-y-1">
                  {form.matchMetadata.map((row, idx) => (
                    <div key={idx} className="flex gap-2 items-center">
                      <input
                        className="flex-1 border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                        placeholder="key"
                        value={row.key}
                        onChange={(e) =>
                          setForm((f) => ({
                            ...f,
                            matchMetadata: f.matchMetadata.map((r, i) =>
                              i === idx ? { ...r, key: e.target.value } : r
                            ),
                          }))
                        }
                      />
                      <input
                        className="flex-1 border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                        placeholder="value"
                        value={row.value}
                        onChange={(e) =>
                          setForm((f) => ({
                            ...f,
                            matchMetadata: f.matchMetadata.map((r, i) =>
                              i === idx ? { ...r, value: e.target.value } : r
                            ),
                          }))
                        }
                      />
                      <button
                        onClick={() =>
                          setForm((f) => ({
                            ...f,
                            matchMetadata: f.matchMetadata.filter((_, i) => i !== idx),
                          }))
                        }
                        className="text-slate-400 hover:text-red-500 text-sm px-1"
                      >
                        ×
                      </button>
                    </div>
                  ))}
                </div>
                <button
                  onClick={() =>
                    setForm((f) => ({
                      ...f,
                      matchMetadata: [...f.matchMetadata, { key: "", value: "" }],
                    }))
                  }
                  className="mt-1 text-xs text-brand-600 hover:underline"
                >
                  + Add Metadata
                </button>
              </div>
              {/* UUID fields */}
              <div className="grid grid-cols-2 gap-3">
                {(
                  [
                    ["matchKeyId", "Key ID"],
                    ["matchUserId", "User ID"],
                    ["matchTeamId", "Team ID"],
                    ["matchOrgId", "Org ID"],
                  ] as [keyof RuleFormState, string][]
                ).map(([field, label]) => (
                  <label key={field} className="block">
                    <span className="text-xs font-medium text-slate-600">{label}</span>
                    <input
                      className={`mt-1 block w-full border rounded-lg px-2 py-1.5 text-sm ${
                        !isValidUuid(form[field] as string)
                          ? "border-red-400"
                          : "border-slate-300"
                      }`}
                      placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
                      value={form[field] as string}
                      onChange={(e) =>
                        setForm((f) => ({ ...f, [field]: e.target.value }))
                      }
                    />
                    {!isValidUuid(form[field] as string) && (
                      <p className="text-xs text-red-500 mt-0.5">Invalid UUID format</p>
                    )}
                  </label>
                ))}
              </div>
            </div>
          )}
        </div>

        {/* Strategy & Targets */}
        <div className="space-y-3">
          <label className="block">
            <span className="text-xs font-medium text-slate-600">Strategy</span>
            <select
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm bg-white"
              value={form.strategy}
              onChange={(e) => onStrategyChange(e.target.value)}
            >
              <option value="direct">direct</option>
              <option value="weighted">weighted</option>
              <option value="least_cost">least_cost</option>
              <option value="failover">failover</option>
              <option value="quality_first">quality_first</option>
            </select>
          </label>

          <div>
            <p className="text-xs font-medium text-slate-600 mb-2">
              Targets{" "}
              {isWeighted && (
                <span
                  className={`ml-1 text-xs ${weightValid ? "text-slate-400" : "text-red-500 font-medium"}`}
                >
                  (weights must sum to 100 — current: {totalWeight})
                </span>
              )}
            </p>
            <div className="space-y-2">
              {form.targets.map((t, idx) => (
                <div key={idx} className="flex gap-2 items-center">
                  <input
                    className="flex-1 border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                    placeholder="provider"
                    value={t.provider}
                    onChange={(e) => updateTarget(idx, "provider", e.target.value)}
                  />
                  <input
                    className="flex-1 border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                    placeholder="model"
                    value={t.model}
                    onChange={(e) => updateTarget(idx, "model", e.target.value)}
                  />
                  {isWeighted && (
                    <input
                      type="number"
                      min={0}
                      max={100}
                      className="w-20 border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                      placeholder="weight"
                      value={t.weight}
                      onChange={(e) => {
                        const parsed = Number(e.target.value);
                        updateTarget(idx, "weight", isNaN(parsed) ? 0 : parsed);
                      }}
                    />
                  )}
                  <button
                    onClick={() => removeTarget(idx)}
                    disabled={form.targets.length <= minTargets}
                    className="text-slate-400 hover:text-red-500 disabled:opacity-30 text-sm px-1"
                    title="Remove"
                  >
                    ×
                  </button>
                </div>
              ))}
            </div>
            <button
              onClick={addTarget}
              disabled={isDirect}
              className="mt-2 text-xs text-brand-600 hover:underline disabled:opacity-40 disabled:cursor-not-allowed"
            >
              + Add Target
            </button>
          </div>
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <button
            className="px-4 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg"
            onClick={onClose}
          >
            Cancel
          </button>
          <button
            disabled={!canSubmit}
            onClick={() => mutation.mutate()}
            className="px-4 py-2 text-sm bg-slate-900 text-white rounded-lg disabled:opacity-50 hover:bg-slate-700"
          >
            {mutation.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// RuleTable
// ---------------------------------------------------------------------------

function RuleTable({
  rules,
  onEdit,
  onDelete,
  onToggle,
}: {
  rules: RoutingRule[];
  onEdit: (rule: RoutingRule) => void;
  onDelete: (rule: RoutingRule) => void;
  onToggle: (rule: RoutingRule) => void;
}) {
  if (rules.length === 0) {
    return (
      <div className="bg-white rounded-xl border border-slate-200 p-8 text-center">
        <p className="text-sm text-slate-400">No routing rules</p>
      </div>
    );
  }

  return (
    <div className="bg-white rounded-xl border border-slate-200 shadow-sm overflow-hidden">
      <table className="w-full text-sm">
        <thead className="bg-slate-50 border-b border-slate-200">
          <tr>
            {["Priority", "Name", "Enabled", "Match", "Strategy", "Targets", "Actions"].map(
              (col) => (
                <th
                  key={col}
                  className="text-left px-4 py-3 font-medium text-slate-600 text-xs uppercase tracking-wide"
                >
                  {col}
                </th>
              )
            )}
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-100">
          {rules.map((rule) => (
            <tr key={rule.id} className="hover:bg-slate-50">
              <td className="px-4 py-3 text-slate-500 font-mono text-xs">{rule.priority}</td>
              <td className="px-4 py-3 font-medium text-slate-900">{rule.name}</td>
              <td className="px-4 py-3">
                <EnabledBadge enabled={rule.enabled} />
              </td>
              <td className="px-4 py-3">
                <MatchSummary match={rule.match} />
              </td>
              <td className="px-4 py-3">
                <StrategyBadge strategy={rule.strategy} />
              </td>
              <td className="px-4 py-3 text-xs text-slate-500">
                {rule.targets.length} target{rule.targets.length !== 1 ? "s" : ""}
              </td>
              <td className="px-4 py-3">
                <div className="flex gap-1.5">
                  <button
                    onClick={() => onToggle(rule)}
                    className="text-xs px-2 py-1 rounded bg-slate-100 text-slate-600 hover:bg-slate-200"
                  >
                    {rule.enabled ? "Disable" : "Enable"}
                  </button>
                  <button
                    onClick={() => onEdit(rule)}
                    className="text-xs px-2 py-1 rounded bg-blue-50 text-blue-700 hover:bg-blue-100"
                  >
                    Edit
                  </button>
                  <button
                    onClick={() => onDelete(rule)}
                    className="text-xs px-2 py-1 rounded bg-red-50 text-red-600 hover:bg-red-100"
                  >
                    Delete
                  </button>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ---------------------------------------------------------------------------
// RoutingPage
// ---------------------------------------------------------------------------

export default function RoutingPage() {
  const qc = useQueryClient();
  const [editingRule, setEditingRule] = useState<RoutingRule | null | undefined>(
    undefined
  ); // undefined=closed, null=new, RoutingRule=edit
  const [deletingRule, setDeletingRule] = useState<RoutingRule | null>(null);
  const [reloadMsg, setReloadMsg] = useState<string | null>(null);
  const [reloadError, setReloadError] = useState<string | null>(null);

  const {
    data: rawRules = [],
    isLoading,
    error: listError,
  } = useQuery<RoutingRule[]>({
    queryKey: ["routing-rules"],
    queryFn: routingRules.list,
  });

  const rules = [...rawRules].sort((a, b) => a.priority - b.priority);

  const reloadMutation = useMutation({
    mutationFn: routingRules.reload,
    onSuccess: () => {
      setReloadMsg("Configuration reloaded");
      setReloadError(null);
      setTimeout(() => setReloadMsg(null), 3000);
    },
    onError: (e: Error) => {
      setReloadError(e.message);
      setTimeout(() => setReloadError(null), 3000);
    },
  });

  const toggleMutation = useMutation({
    mutationFn: (rule: RoutingRule) => {
      const payload: CreateRoutingRulePayload = {
        name: rule.name,
        priority: rule.priority,
        enabled: !rule.enabled,
        match: rule.match,
        strategy: rule.strategy,
        targets: rule.targets,
      };
      return routingRules.update(rule.id, payload);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["routing-rules"] }),
    onError: (e: Error) => setError(e.message),
  });

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Routing</h1>
          <p className="text-sm text-slate-500 mt-1">
            {rules.length} rule{rules.length !== 1 ? "s" : ""}
          </p>
        </div>
        <div className="flex items-center gap-3">
          {reloadMsg && (
            <span className="text-xs text-green-700 font-medium">{reloadMsg}</span>
          )}
          {reloadError && (
            <span className="text-xs text-red-600">{reloadError}</span>
          )}
          <button
            onClick={() => reloadMutation.mutate()}
            disabled={reloadMutation.isPending}
            className="px-3 py-2 text-sm text-slate-600 border border-slate-300 rounded-lg hover:bg-slate-50 disabled:opacity-50"
          >
            {reloadMutation.isPending ? "Reloading…" : "Reload"}
          </button>
          <button
            onClick={() => setEditingRule(null)}
            className="bg-brand-600 text-white px-4 py-2 rounded-lg text-sm font-medium hover:bg-brand-700"
          >
            + Add Rule
          </button>
        </div>
      </div>

      {listError && (
        <p className="text-sm text-red-600 bg-red-50 rounded p-3">
          {(listError as Error).message}
        </p>
      )}

      {/* Rule Table */}
      {isLoading ? (
        <p className="text-sm text-slate-400">Loading…</p>
      ) : (
        <RuleTable
          rules={rules}
          onEdit={(rule) => setEditingRule(rule)}
          onDelete={(rule) => setDeletingRule(rule)}
          onToggle={(rule) => toggleMutation.mutate(rule)}
        />
      )}

      {/* DryRun Panel */}
      <DryRunPanel rules={rules} />

      {/* Modals */}
      {editingRule !== undefined && (
        <RuleDialog
          editingRule={editingRule}
          onClose={() => setEditingRule(undefined)}
          onSaved={() => setEditingRule(undefined)}
        />
      )}
      {deletingRule && (
        <DeleteConfirmDialog
          rule={deletingRule}
          onClose={() => setDeletingRule(null)}
          onDeleted={() => setDeletingRule(null)}
        />
      )}
    </div>
  );
}

"use client";

import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { guardrails, providers, GuardrailPolicy, UpdateGuardrailPayload } from "@/lib/api";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const ACTION_OPTIONS = ["block", "mask", "log_only"];
const ENGINE_OPTIONS = ["regex", "llm"];
const PII_CATEGORIES = [
  "credit_card",
  "ssn",
  "email",
  "phone_us",
  "ip_address",
  "korean_rrn",
];
const CONTENT_CATEGORIES = ["hate", "violence", "sexual"];

function Toggle({
  checked,
  onChange,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <button
      type="button"
      onClick={() => onChange(!checked)}
      className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
        checked ? "bg-brand-600" : "bg-slate-300"
      }`}
    >
      <span
        className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${
          checked ? "translate-x-6" : "translate-x-1"
        }`}
      />
    </button>
  );
}

function Select({
  value,
  options,
  onChange,
}: {
  value: string;
  options: string[];
  onChange: (v: string) => void;
}) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="rounded border border-slate-300 bg-white px-2 py-1 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
    >
      {options.map((o) => (
        <option key={o} value={o}>
          {o.replace("_", " ")}
        </option>
      ))}
    </select>
  );
}

function SectionCard({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border border-slate-200 bg-white shadow-sm">
      <div className="border-b border-slate-100 px-6 py-4">
        <h2 className="text-base font-semibold text-slate-900">{title}</h2>
        {description && (
          <p className="mt-0.5 text-sm text-slate-500">{description}</p>
        )}
      </div>
      <div className="px-6 py-5">{children}</div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Per-guardrail card components
// ---------------------------------------------------------------------------

function LLMJudgeCard({ policy }: { policy: GuardrailPolicy }) {
  const qc = useQueryClient();
  const cfg = policy.config_json as { provider?: string; model?: string };
  const [selectedProvider, setSelectedProvider] = useState(cfg.provider ?? "");
  const [selectedModel, setSelectedModel] = useState(cfg.model ?? "");

  useEffect(() => {
    const c = policy.config_json as { provider?: string; model?: string };
    setSelectedProvider(c.provider ?? "");
    setSelectedModel(c.model ?? "");
  }, [policy.config_json]);

  const { data: providerList } = useQuery({
    queryKey: ["providers"],
    queryFn: providers.list,
  });

  const { data: modelList } = useQuery({
    queryKey: ["provider-models", selectedProvider],
    queryFn: () => {
      const p = providerList?.find((p) => p.name === selectedProvider);
      return p ? providers.models.list(p.id) : Promise.resolve([]);
    },
    enabled: !!selectedProvider && !!providerList,
  });

  const mutation = useMutation({
    mutationFn: (payload: UpdateGuardrailPayload) =>
      guardrails.update("llm_judge", payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["guardrails"] }),
  });

  const enabledProviders = providerList?.filter((p) => p.is_enabled) ?? [];
  const enabledModels = modelList?.filter((m) => m.is_enabled) ?? [];

  return (
    <SectionCard
      title="LLM Judge"
      description='Uses a registered provider model to evaluate prompt safety and content policy. Applied when engine is set to "llm" on other guardrails.'
    >
      <div className="space-y-4">
        <div className="grid grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-slate-700 mb-1">
              Provider
            </label>
            <select
              value={selectedProvider}
              onChange={(e) => {
                setSelectedProvider(e.target.value);
                setSelectedModel("");
              }}
              className="w-full rounded border border-slate-300 bg-white px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
            >
              <option value="">— select provider —</option>
              {enabledProviders.map((p) => (
                <option key={p.id} value={p.name}>
                  {p.display_name || p.name}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="block text-sm font-medium text-slate-700 mb-1">
              Model
            </label>
            <select
              value={selectedModel}
              onChange={(e) => setSelectedModel(e.target.value)}
              disabled={!selectedProvider}
              className="w-full rounded border border-slate-300 bg-white px-2 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500 disabled:bg-slate-50 disabled:text-slate-400"
            >
              <option value="">— select model —</option>
              {enabledModels.map((m) => (
                <option key={m.id} value={m.model_id}>
                  {m.display_name || m.model_id}
                </option>
              ))}
            </select>
          </div>
        </div>
        <div className="flex justify-end">
          <button
            onClick={() =>
              mutation.mutate({
                config_json: { provider: selectedProvider, model: selectedModel },
              })
            }
            disabled={mutation.isPending || !selectedProvider || !selectedModel}
            className="rounded bg-brand-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-50"
          >
            {mutation.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </SectionCard>
  );
}

function PIICard({ policy }: { policy: GuardrailPolicy }) {
  const qc = useQueryClient();
  const cfg = policy.config_json as { categories?: string[] };
  const [enabled, setEnabled] = useState(policy.is_enabled);
  const [action, setAction] = useState(policy.action);
  const [categories, setCategories] = useState<string[]>(
    cfg.categories ?? PII_CATEGORIES
  );

  useEffect(() => {
    setEnabled(policy.is_enabled);
    setAction(policy.action);
    setCategories((policy.config_json as { categories?: string[] }).categories ?? PII_CATEGORIES);
  }, [policy.is_enabled, policy.action, policy.config_json]);

  const mutation = useMutation({
    mutationFn: (payload: UpdateGuardrailPayload) =>
      guardrails.update("pii", payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["guardrails"] }),
  });

  function toggleCategory(cat: string) {
    setCategories((prev) =>
      prev.includes(cat) ? prev.filter((c) => c !== cat) : [...prev, cat]
    );
  }

  return (
    <SectionCard
      title="PII Detection"
      description="Detects and masks personally identifiable information in both requests and responses."
    >
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium text-slate-700">Enabled</span>
          <Toggle checked={enabled} onChange={setEnabled} />
        </div>
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium text-slate-700">Action</span>
          <Select value={action} options={ACTION_OPTIONS} onChange={setAction} />
        </div>
        <div>
          <p className="text-sm font-medium text-slate-700 mb-2">Categories</p>
          <div className="flex flex-wrap gap-2">
            {PII_CATEGORIES.map((cat) => (
              <label key={cat} className="flex items-center gap-1.5 text-sm cursor-pointer">
                <input
                  type="checkbox"
                  checked={categories.includes(cat)}
                  onChange={() => toggleCategory(cat)}
                  className="h-3.5 w-3.5 rounded border-slate-300 text-brand-600"
                />
                {cat.replace(/_/g, " ")}
              </label>
            ))}
          </div>
        </div>
        <div className="flex justify-end">
          <button
            onClick={() =>
              mutation.mutate({
                is_enabled: enabled,
                action,
                config_json: { categories },
              })
            }
            disabled={mutation.isPending}
            className="rounded bg-brand-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-50"
          >
            {mutation.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </SectionCard>
  );
}

function PromptInjectionCard({ policy }: { policy: GuardrailPolicy }) {
  const qc = useQueryClient();
  const [enabled, setEnabled] = useState(policy.is_enabled);
  const [action, setAction] = useState(policy.action);
  const [engine, setEngine] = useState(policy.engine ?? "regex");

  useEffect(() => {
    setEnabled(policy.is_enabled);
    setAction(policy.action);
    setEngine(policy.engine ?? "regex");
  }, [policy.is_enabled, policy.action, policy.engine]);

  const mutation = useMutation({
    mutationFn: (payload: UpdateGuardrailPayload) =>
      guardrails.update("prompt_injection", payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["guardrails"] }),
  });

  return (
    <SectionCard
      title="Prompt Injection"
      description="Detects attempts to hijack or override system instructions in user messages."
    >
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium text-slate-700">Enabled</span>
          <Toggle checked={enabled} onChange={setEnabled} />
        </div>
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium text-slate-700">Action</span>
          <Select value={action} options={ACTION_OPTIONS} onChange={setAction} />
        </div>
        <div>
          <p className="text-sm font-medium text-slate-700 mb-2">Engine</p>
          <div className="flex gap-4">
            {ENGINE_OPTIONS.map((opt) => (
              <label key={opt} className="flex items-center gap-1.5 text-sm cursor-pointer">
                <input
                  type="radio"
                  checked={engine === opt}
                  onChange={() => setEngine(opt)}
                  className="text-brand-600"
                />
                {opt}
              </label>
            ))}
          </div>
        </div>
        <div className="flex justify-end">
          <button
            onClick={() =>
              mutation.mutate({ is_enabled: enabled, action, engine })
            }
            disabled={mutation.isPending}
            className="rounded bg-brand-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-50"
          >
            {mutation.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </SectionCard>
  );
}

function ContentFilterCard({ policy }: { policy: GuardrailPolicy }) {
  const qc = useQueryClient();
  const cfg = policy.config_json as { categories?: string[] };
  const [enabled, setEnabled] = useState(policy.is_enabled);
  const [action, setAction] = useState(policy.action);
  const [engine, setEngine] = useState(policy.engine ?? "regex");
  const [categories, setCategories] = useState<string[]>(
    cfg.categories ?? CONTENT_CATEGORIES
  );

  useEffect(() => {
    setEnabled(policy.is_enabled);
    setAction(policy.action);
    setEngine(policy.engine ?? "regex");
    setCategories((policy.config_json as { categories?: string[] }).categories ?? CONTENT_CATEGORIES);
  }, [policy.is_enabled, policy.action, policy.engine, policy.config_json]);

  const mutation = useMutation({
    mutationFn: (payload: UpdateGuardrailPayload) =>
      guardrails.update("content_filter", payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["guardrails"] }),
  });

  function toggleCategory(cat: string) {
    setCategories((prev) =>
      prev.includes(cat) ? prev.filter((c) => c !== cat) : [...prev, cat]
    );
  }

  return (
    <SectionCard
      title="Content Filter"
      description="Filters harmful content (hate speech, violence, sexual content) in both requests and responses."
    >
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium text-slate-700">Enabled</span>
          <Toggle checked={enabled} onChange={setEnabled} />
        </div>
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium text-slate-700">Action</span>
          <Select value={action} options={ACTION_OPTIONS} onChange={setAction} />
        </div>
        <div>
          <p className="text-sm font-medium text-slate-700 mb-2">Engine</p>
          <div className="flex gap-4">
            {ENGINE_OPTIONS.map((opt) => (
              <label key={opt} className="flex items-center gap-1.5 text-sm cursor-pointer">
                <input
                  type="radio"
                  checked={engine === opt}
                  onChange={() => setEngine(opt)}
                  className="text-brand-600"
                />
                {opt}
              </label>
            ))}
          </div>
        </div>
        <div>
          <p className="text-sm font-medium text-slate-700 mb-2">Categories</p>
          <div className="flex flex-wrap gap-4">
            {CONTENT_CATEGORIES.map((cat) => (
              <label key={cat} className="flex items-center gap-1.5 text-sm cursor-pointer">
                <input
                  type="checkbox"
                  checked={categories.includes(cat)}
                  onChange={() => toggleCategory(cat)}
                  className="h-3.5 w-3.5 rounded border-slate-300 text-brand-600"
                />
                {cat}
              </label>
            ))}
          </div>
        </div>
        <div className="flex justify-end">
          <button
            onClick={() =>
              mutation.mutate({
                is_enabled: enabled,
                action,
                engine,
                config_json: { categories },
              })
            }
            disabled={mutation.isPending}
            className="rounded bg-brand-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-50"
          >
            {mutation.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </SectionCard>
  );
}

function CustomKeywordsCard({ policy }: { policy: GuardrailPolicy }) {
  const qc = useQueryClient();
  const cfg = policy.config_json as { blocked?: string[] };
  const [enabled, setEnabled] = useState(policy.is_enabled);
  const [action, setAction] = useState(policy.action);
  const [keywords, setKeywords] = useState(
    (cfg.blocked ?? []).join("\n")
  );

  useEffect(() => {
    setEnabled(policy.is_enabled);
    setAction(policy.action);
    setKeywords(((policy.config_json as { blocked?: string[] }).blocked ?? []).join("\n"));
  }, [policy.is_enabled, policy.action, policy.config_json]);

  const mutation = useMutation({
    mutationFn: (payload: UpdateGuardrailPayload) =>
      guardrails.update("custom_keywords", payload),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["guardrails"] }),
  });

  const blocked = keywords
    .split("\n")
    .map((k) => k.trim())
    .filter(Boolean);

  return (
    <SectionCard
      title="Custom Keywords"
      description="Blocks or logs requests and responses that contain any of the specified keywords."
    >
      <div className="space-y-4">
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium text-slate-700">Enabled</span>
          <Toggle checked={enabled} onChange={setEnabled} />
        </div>
        <div className="flex items-center gap-3">
          <span className="text-sm font-medium text-slate-700">Action</span>
          <Select value={action} options={ACTION_OPTIONS} onChange={setAction} />
        </div>
        <div>
          <label className="block text-sm font-medium text-slate-700 mb-1">
            Blocked keywords
            <span className="ml-1 text-xs text-slate-400">(one per line)</span>
          </label>
          <textarea
            value={keywords}
            onChange={(e) => setKeywords(e.target.value)}
            rows={5}
            placeholder="keyword1&#10;keyword2&#10;secret phrase"
            className="w-full rounded border border-slate-300 px-3 py-2 text-sm font-mono focus:outline-none focus:ring-2 focus:ring-brand-500"
          />
          <p className="mt-1 text-xs text-slate-400">
            {blocked.length} keyword{blocked.length !== 1 ? "s" : ""} configured
          </p>
        </div>
        <div className="flex justify-end">
          <button
            onClick={() =>
              mutation.mutate({
                is_enabled: enabled,
                action,
                config_json: { blocked },
              })
            }
            disabled={mutation.isPending}
            className="rounded bg-brand-600 px-4 py-1.5 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-50"
          >
            {mutation.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </SectionCard>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function GuardrailsPage() {
  const { data: policies, isLoading, error } = useQuery({
    queryKey: ["guardrails"],
    queryFn: guardrails.list,
  });

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64 text-slate-400 text-sm">
        Loading guardrails…
      </div>
    );
  }

  if (error || !policies) {
    return (
      <div className="flex items-center justify-center h-64 text-red-500 text-sm">
        Failed to load guardrail policies.
      </div>
    );
  }

  const byType = Object.fromEntries(
    policies.map((p) => [p.guardrail_type, p])
  );

  const pii = byType["pii"];
  const injection = byType["prompt_injection"];
  const contentFilter = byType["content_filter"];
  const keywords = byType["custom_keywords"];
  const llmJudge = byType["llm_judge"];

  return (
    <div className="max-w-3xl mx-auto space-y-6 py-8 px-4">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">Guardrails</h1>
        <p className="mt-1 text-sm text-slate-500">
          Configure content safety policies. Changes take effect immediately without server restart.
        </p>
      </div>

      {llmJudge && <LLMJudgeCard policy={llmJudge} />}
      {pii && <PIICard policy={pii} />}
      {injection && <PromptInjectionCard policy={injection} />}
      {contentFilter && <ContentFilterCard policy={contentFilter} />}
      {keywords && <CustomKeywordsCard policy={keywords} />}
    </div>
  );
}

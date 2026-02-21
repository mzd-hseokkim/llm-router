"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  providers,
  providerKeys,
  Provider,
  Model,
  ProviderKey,
  CreateProviderPayload,
  CreateModelPayload,
  UpdateModelPayload,
} from "@/lib/api";

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const PROVIDER_COLORS: Record<string, string> = {
  openai: "bg-green-100 text-green-800",
  anthropic: "bg-orange-100 text-orange-800",
  google: "bg-blue-100 text-blue-800",
  gemini: "bg-blue-100 text-blue-800",
  grok: "bg-gray-100 text-gray-800",
  azure: "bg-sky-100 text-sky-800",
  mistral: "bg-purple-100 text-purple-800",
  cohere: "bg-pink-100 text-pink-800",
  bedrock: "bg-yellow-100 text-yellow-800",
};

const ADAPTER_TYPES = [
  "openai",
  "anthropic",
  "google",
  "grok",
  "mistral",
  "cohere",
  "bedrock",
  "azure",
  "selfhosted",
];

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

function ProviderBadge({ name }: { name: string }) {
  const cls =
    PROVIDER_COLORS[name.toLowerCase()] ?? "bg-slate-100 text-slate-700";
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${cls}`}
    >
      {name}
    </span>
  );
}

function StatusDot({ active }: { active: boolean }) {
  return (
    <span
      className={`inline-flex items-center gap-1 text-xs font-medium ${
        active ? "text-green-700" : "text-slate-400"
      }`}
    >
      <span
        className={`w-1.5 h-1.5 rounded-full ${
          active ? "bg-green-500" : "bg-slate-300"
        }`}
      />
      {active ? "Enabled" : "Disabled"}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Add Provider Dialog
// ---------------------------------------------------------------------------

function AddProviderDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: () => void;
}) {
  const [form, setForm] = useState<CreateProviderPayload>({
    name: "",
    adapter_type: "openai",
    display_name: "",
    base_url: "",
    is_enabled: true,
  });
  const [apiKey, setApiKey] = useState("");
  const [error, setError] = useState("");

  const mutation = useMutation({
    mutationFn: () =>
      providers.create({ ...form, api_key: apiKey.trim() || undefined }),
    onSuccess: () => {
      onCreated();
      onClose();
    },
    onError: (e: Error) => setError(e.message),
  });

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-md p-6 space-y-4">
        <h2 className="text-lg font-semibold text-slate-900">Add Provider</h2>

        {error && (
          <p className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</p>
        )}

        <div className="space-y-3">
          <label className="block">
            <span className="text-xs font-medium text-slate-600">
              Routing Name *
            </span>
            <input
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              placeholder="openai, my-custom-provider, ..."
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
            />
          </label>

          <label className="block">
            <span className="text-xs font-medium text-slate-600">
              Adapter Type *
            </span>
            <select
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              value={form.adapter_type}
              onChange={(e) =>
                setForm({ ...form, adapter_type: e.target.value })
              }
            >
              {ADAPTER_TYPES.map((t) => (
                <option key={t} value={t}>
                  {t}
                </option>
              ))}
            </select>
          </label>

          <label className="block">
            <span className="text-xs font-medium text-slate-600">
              Display Name
            </span>
            <input
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              value={form.display_name ?? ""}
              onChange={(e) =>
                setForm({ ...form, display_name: e.target.value })
              }
            />
          </label>

          <label className="block">
            <span className="text-xs font-medium text-slate-600">
              Base URL Override
            </span>
            <input
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm font-mono"
              placeholder="https://..."
              value={form.base_url ?? ""}
              onChange={(e) => setForm({ ...form, base_url: e.target.value })}
            />
          </label>

          <label className="block">
            <span className="text-xs font-medium text-slate-600">
              API Key
            </span>
            <input
              type="password"
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm font-mono"
              placeholder="sk-..."
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
            />
            <p className="mt-1 text-xs text-slate-400">
              Optional. Registers as the default key for this provider.
            </p>
          </label>

          <label className="flex items-center gap-2 cursor-pointer">
            <input
              type="checkbox"
              checked={form.is_enabled ?? true}
              onChange={(e) =>
                setForm({ ...form, is_enabled: e.target.checked })
              }
            />
            <span className="text-sm text-slate-700">Enabled</span>
          </label>
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <button
            className="px-4 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg"
            onClick={onClose}
          >
            Cancel
          </button>
          <button
            className="px-4 py-2 text-sm bg-slate-900 text-white rounded-lg disabled:opacity-50"
            disabled={mutation.isPending || !form.name || !form.adapter_type}
            onClick={() => mutation.mutate()}
          >
            {mutation.isPending ? "Creating…" : "Create"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Add / Edit Model Dialog
// ---------------------------------------------------------------------------

function ModelDialog({
  providerId,
  existing,
  onClose,
  onSaved,
}: {
  providerId: string;
  existing?: Model;
  onClose: () => void;
  onSaved: () => void;
}) {
  const isEdit = !!existing;
  const [form, setForm] = useState<CreateModelPayload>({
    model_id: existing?.model_id ?? "",
    model_name: existing?.model_name ?? "",
    display_name: existing?.display_name ?? "",
    is_enabled: existing?.is_enabled ?? true,
    input_per_million_tokens: existing?.input_per_million_tokens ?? 0,
    output_per_million_tokens: existing?.output_per_million_tokens ?? 0,
    context_window: existing?.context_window,
    max_output_tokens: existing?.max_output_tokens,
    supports_streaming: existing?.supports_streaming ?? true,
    supports_tools: existing?.supports_tools ?? false,
    supports_vision: existing?.supports_vision ?? false,
    tags: existing?.tags ?? [],
  });
  const [error, setError] = useState("");

  const mutation = useMutation({
    mutationFn: () =>
      isEdit
        ? providers.models.update(providerId, existing!.id, form as UpdateModelPayload)
        : providers.models.create(providerId, form),
    onSuccess: () => {
      onSaved();
      onClose();
    },
    onError: (e: Error) => setError(e.message),
  });

  function setNum(field: keyof CreateModelPayload, val: string) {
    const n = parseFloat(val);
    setForm({ ...form, [field]: isNaN(n) ? undefined : n });
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-lg p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <h2 className="text-lg font-semibold text-slate-900">
          {isEdit ? "Edit Model" : "Add Model"}
        </h2>

        {error && (
          <p className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</p>
        )}

        <div className="grid grid-cols-2 gap-3">
          <label className="block col-span-2">
            <span className="text-xs font-medium text-slate-600">
              Model ID * (e.g. openai/gpt-4o)
            </span>
            <input
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm font-mono"
              value={form.model_id}
              disabled={isEdit}
              onChange={(e) => setForm({ ...form, model_id: e.target.value })}
            />
          </label>

          <label className="block col-span-2">
            <span className="text-xs font-medium text-slate-600">
              Upstream Model Name * (sent to API)
            </span>
            <input
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm font-mono"
              value={form.model_name}
              onChange={(e) =>
                setForm({ ...form, model_name: e.target.value })
              }
            />
          </label>

          <label className="block col-span-2">
            <span className="text-xs font-medium text-slate-600">
              Display Name
            </span>
            <input
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              value={form.display_name ?? ""}
              onChange={(e) =>
                setForm({ ...form, display_name: e.target.value })
              }
            />
          </label>

          <label className="block">
            <span className="text-xs font-medium text-slate-600">
              Input $/M tokens
            </span>
            <input
              type="number"
              step="0.0001"
              min="0"
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              value={form.input_per_million_tokens ?? ""}
              onChange={(e) =>
                setNum("input_per_million_tokens", e.target.value)
              }
            />
          </label>

          <label className="block">
            <span className="text-xs font-medium text-slate-600">
              Output $/M tokens
            </span>
            <input
              type="number"
              step="0.0001"
              min="0"
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              value={form.output_per_million_tokens ?? ""}
              onChange={(e) =>
                setNum("output_per_million_tokens", e.target.value)
              }
            />
          </label>

          <label className="block">
            <span className="text-xs font-medium text-slate-600">
              Context Window
            </span>
            <input
              type="number"
              min="0"
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              value={form.context_window ?? ""}
              onChange={(e) => setNum("context_window", e.target.value)}
            />
          </label>

          <label className="block">
            <span className="text-xs font-medium text-slate-600">
              Max Output Tokens
            </span>
            <input
              type="number"
              min="0"
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              value={form.max_output_tokens ?? ""}
              onChange={(e) => setNum("max_output_tokens", e.target.value)}
            />
          </label>
        </div>

        <div className="flex flex-wrap gap-4">
          {(
            [
              ["is_enabled", "Enabled"],
              ["supports_streaming", "Streaming"],
              ["supports_tools", "Tool Use"],
              ["supports_vision", "Vision"],
            ] as [keyof CreateModelPayload, string][]
          ).map(([field, label]) => (
            <label key={field} className="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={!!form[field]}
                onChange={(e) =>
                  setForm({ ...form, [field]: e.target.checked })
                }
              />
              <span className="text-sm text-slate-700">{label}</span>
            </label>
          ))}
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <button
            className="px-4 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg"
            onClick={onClose}
          >
            Cancel
          </button>
          <button
            className="px-4 py-2 text-sm bg-slate-900 text-white rounded-lg disabled:opacity-50"
            disabled={
              mutation.isPending || !form.model_id || !form.model_name
            }
            onClick={() => mutation.mutate()}
          >
            {mutation.isPending ? "Saving…" : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Provider Card
// ---------------------------------------------------------------------------

function ProviderCard({ prov }: { prov: Provider }) {
  const qc = useQueryClient();
  const [expanded, setExpanded] = useState(false);
  const [addModel, setAddModel] = useState(false);
  const [editModel, setEditModel] = useState<Model | null>(null);

  const { data: modelList = [] } = useQuery({
    queryKey: ["provider-models", prov.id],
    queryFn: () => providers.models.list(prov.id),
    enabled: expanded,
  });

  const { data: keyList = [] } = useQuery({
    queryKey: ["provider-keys", prov.name],
    queryFn: () => providerKeys.list(prov.name),
    enabled: expanded,
  });

  const deleteProvider = useMutation({
    mutationFn: () => providers.delete(prov.id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["providers"] }),
  });

  const toggleEnabled = useMutation({
    mutationFn: () =>
      providers.update(prov.id, { is_enabled: !prov.is_enabled }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["providers"] }),
  });

  const deleteModel = useMutation({
    mutationFn: (modelId: string) =>
      providers.models.delete(prov.id, modelId),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["provider-models", prov.id] });
      qc.invalidateQueries({ queryKey: ["providers"] });
    },
  });

  function refreshModels() {
    qc.invalidateQueries({ queryKey: ["provider-models", prov.id] });
    qc.invalidateQueries({ queryKey: ["providers"] });
  }

  return (
    <>
      <div className="bg-white rounded-xl border border-slate-200 shadow-sm overflow-hidden">
        {/* Header */}
        <div className="px-5 py-3 bg-slate-50 border-b border-slate-200 flex items-center gap-3">
          <ProviderBadge name={prov.name} />
          {prov.display_name && (
            <span className="text-sm font-medium text-slate-700">
              {prov.display_name}
            </span>
          )}
          <span className="text-xs text-slate-400">{prov.adapter_type}</span>
          {prov.base_url && (
            <span className="text-xs font-mono text-slate-400 truncate max-w-xs">
              {prov.base_url}
            </span>
          )}
          <div className="ml-auto flex items-center gap-2">
            <StatusDot active={prov.is_enabled} />
            <button
              className="text-xs text-slate-500 hover:text-slate-900 border border-slate-200 rounded px-2 py-1"
              onClick={() => toggleEnabled.mutate()}
            >
              {prov.is_enabled ? "Disable" : "Enable"}
            </button>
            <button
              className="text-xs text-slate-500 hover:text-red-600 border border-slate-200 rounded px-2 py-1"
              onClick={() => {
                if (confirm(`Delete provider "${prov.name}"?`))
                  deleteProvider.mutate();
              }}
            >
              Delete
            </button>
            <button
              className="text-xs text-slate-500 hover:text-slate-900 border border-slate-200 rounded px-2 py-1"
              onClick={() => setExpanded((v) => !v)}
            >
              {expanded ? "▲ Collapse" : "▼ Expand"}
            </button>
          </div>
        </div>

        {/* Expanded content */}
        {expanded && (
          <div className="divide-y divide-slate-100">
            {/* Models section */}
            <div className="p-4">
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-sm font-semibold text-slate-700">
                  Models ({modelList.length})
                </h3>
                <button
                  className="text-xs bg-slate-900 text-white px-3 py-1 rounded-lg"
                  onClick={() => setAddModel(true)}
                >
                  + Add Model
                </button>
              </div>

              {modelList.length === 0 ? (
                <p className="text-xs text-slate-400">
                  No models configured. Click "Add Model" to add one.
                </p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-xs">
                    <thead>
                      <tr className="border-b border-slate-100 text-slate-500">
                        {[
                          "Model ID",
                          "Upstream Name",
                          "Input $/M",
                          "Output $/M",
                          "Ctx",
                          "Caps",
                          "Status",
                          "",
                        ].map((h) => (
                          <th
                            key={h}
                            className="text-left px-2 py-1.5 font-medium"
                          >
                            {h}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-slate-50">
                      {modelList.map((m: Model) => (
                        <tr key={m.id} className="hover:bg-slate-50">
                          <td className="px-2 py-2 font-mono text-slate-700">
                            {m.model_id}
                          </td>
                          <td className="px-2 py-2 font-mono text-slate-500">
                            {m.model_name}
                          </td>
                          <td className="px-2 py-2 text-slate-600">
                            ${m.input_per_million_tokens.toFixed(2)}
                          </td>
                          <td className="px-2 py-2 text-slate-600">
                            ${m.output_per_million_tokens.toFixed(2)}
                          </td>
                          <td className="px-2 py-2 text-slate-500">
                            {m.context_window
                              ? `${(m.context_window / 1000).toFixed(0)}k`
                              : "—"}
                          </td>
                          <td className="px-2 py-2">
                            <span className="flex gap-1">
                              {m.supports_streaming && (
                                <span title="Streaming" className="text-green-600">S</span>
                              )}
                              {m.supports_tools && (
                                <span title="Tools" className="text-blue-600">T</span>
                              )}
                              {m.supports_vision && (
                                <span title="Vision" className="text-purple-600">V</span>
                              )}
                            </span>
                          </td>
                          <td className="px-2 py-2">
                            <StatusDot active={m.is_enabled} />
                          </td>
                          <td className="px-2 py-2">
                            <span className="flex gap-2">
                              <button
                                className="text-slate-400 hover:text-slate-700"
                                onClick={() => setEditModel(m)}
                              >
                                Edit
                              </button>
                              <button
                                className="text-slate-400 hover:text-red-600"
                                onClick={() => {
                                  if (confirm(`Delete model "${m.model_id}"?`))
                                    deleteModel.mutate(m.id);
                                }}
                              >
                                Del
                              </button>
                            </span>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>

            {/* Provider Keys section */}
            <div className="p-4">
              <h3 className="text-sm font-semibold text-slate-700 mb-3">
                API Keys ({keyList.length})
              </h3>
              {keyList.length === 0 ? (
                <p className="text-xs text-slate-400">
                  No keys configured for this provider.
                </p>
              ) : (
                <div className="overflow-x-auto">
                  <table className="w-full text-xs">
                    <thead>
                      <tr className="border-b border-slate-100 text-slate-500">
                        {["Alias", "Preview", "Status", "Weight", "Use Count", "Monthly Spend"].map(
                          (h) => (
                            <th
                              key={h}
                              className="text-left px-2 py-1.5 font-medium"
                            >
                              {h}
                            </th>
                          )
                        )}
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-slate-50">
                      {keyList.map((pk: ProviderKey) => (
                        <tr key={pk.id} className="hover:bg-slate-50">
                          <td className="px-2 py-2 font-medium text-slate-700">
                            {pk.key_alias}
                          </td>
                          <td className="px-2 py-2 font-mono text-slate-400">
                            {pk.key_preview}
                          </td>
                          <td className="px-2 py-2">
                            <StatusDot active={pk.is_active} />
                          </td>
                          <td className="px-2 py-2 text-slate-500">
                            {pk.weight}
                          </td>
                          <td className="px-2 py-2 text-slate-500">
                            {pk.use_count.toLocaleString()}
                          </td>
                          <td className="px-2 py-2 text-slate-500">
                            ${pk.current_month_spend.toFixed(4)}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
          </div>
        )}
      </div>

      {/* Dialogs */}
      {addModel && (
        <ModelDialog
          providerId={prov.id}
          onClose={() => setAddModel(false)}
          onSaved={refreshModels}
        />
      )}
      {editModel && (
        <ModelDialog
          providerId={prov.id}
          existing={editModel}
          onClose={() => setEditModel(null)}
          onSaved={refreshModels}
        />
      )}
    </>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function ProvidersPage() {
  const qc = useQueryClient();
  const [addProvider, setAddProvider] = useState(false);

  const { data: providerList = [], isLoading } = useQuery({
    queryKey: ["providers"],
    queryFn: () => providers.list(),
    refetchInterval: 30_000,
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Providers</h1>
          <p className="text-sm text-slate-500 mt-1">
            Manage LLM providers and their model catalog
          </p>
        </div>
        <button
          className="bg-slate-900 text-white text-sm px-4 py-2 rounded-lg hover:bg-slate-700"
          onClick={() => setAddProvider(true)}
        >
          + Add Provider
        </button>
      </div>

      {isLoading ? (
        <p className="text-sm text-slate-400">Loading…</p>
      ) : providerList.length === 0 ? (
        <div className="bg-white rounded-xl border border-slate-200 p-8 text-center text-sm text-slate-400">
          No providers configured. Click "Add Provider" to create one, or
          configure providers in config.yaml (they appear here automatically
          when the DB is populated).
        </div>
      ) : (
        <div className="space-y-4">
          {providerList.map((prov: Provider) => (
            <ProviderCard key={prov.id} prov={prov} />
          ))}
        </div>
      )}

      {addProvider && (
        <AddProviderDialog
          onClose={() => setAddProvider(false)}
          onCreated={() => qc.invalidateQueries({ queryKey: ["providers"] })}
        />
      )}
    </div>
  );
}

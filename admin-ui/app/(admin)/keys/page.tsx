"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { keys, CreateKeyPayload, VirtualKey } from "@/lib/api";

function Badge({ active }: { active: boolean }) {
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${
        active ? "bg-green-100 text-green-800" : "bg-slate-100 text-slate-500"
      }`}
    >
      {active ? "Active" : "Inactive"}
    </span>
  );
}

function CreateKeyDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (key: string) => void;
}) {
  const qc = useQueryClient();
  const [form, setForm] = useState<CreateKeyPayload>({ name: "" });
  const [error, setError] = useState("");

  const mutation = useMutation({
    mutationFn: () => keys.create(form),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["keys"] });
      onCreated(data.key);
    },
    onError: (e: Error) => setError(e.message),
  });

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-md p-6 space-y-4">
        <h2 className="text-lg font-semibold">Create Virtual Key</h2>

        {error && <p className="text-sm text-red-600">{error}</p>}

        <div className="space-y-3">
          <label className="block">
            <span className="text-sm font-medium text-slate-700">Name *</span>
            <input
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            />
          </label>
          <label className="block">
            <span className="text-sm font-medium text-slate-700">RPM Limit</span>
            <input
              type="number"
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              placeholder="No limit"
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  rpm_limit: e.target.value ? Number(e.target.value) : undefined,
                }))
              }
            />
          </label>
          <label className="block">
            <span className="text-sm font-medium text-slate-700">TPM Limit</span>
            <input
              type="number"
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              placeholder="No limit"
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  tpm_limit: e.target.value ? Number(e.target.value) : undefined,
                }))
              }
            />
          </label>
          <label className="block">
            <span className="text-sm font-medium text-slate-700">Budget (USD)</span>
            <input
              type="number"
              step="0.01"
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              placeholder="No budget"
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  budget_usd: e.target.value ? Number(e.target.value) : undefined,
                }))
              }
            />
          </label>
        </div>

        <div className="flex gap-3 pt-2">
          <button
            onClick={() => mutation.mutate()}
            disabled={!form.name || mutation.isPending}
            className="flex-1 bg-brand-600 text-white py-2 rounded-lg text-sm font-medium disabled:opacity-50"
          >
            {mutation.isPending ? "Creating…" : "Create Key"}
          </button>
          <button
            onClick={onClose}
            className="flex-1 border border-slate-300 text-slate-700 py-2 rounded-lg text-sm font-medium"
          >
            Cancel
          </button>
        </div>
      </div>
    </div>
  );
}

function NewKeyDisplay({ rawKey, onClose }: { rawKey: string; onClose: () => void }) {
  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-md p-6 space-y-4">
        <h2 className="text-lg font-semibold text-green-700">Key Created!</h2>
        <p className="text-sm text-slate-600">
          Copy this key now — it will <strong>not</strong> be shown again.
        </p>
        <code className="block bg-slate-100 rounded-lg p-3 text-xs break-all">{rawKey}</code>
        <button
          onClick={() => navigator.clipboard.writeText(rawKey)}
          className="w-full border border-slate-300 py-2 rounded-lg text-sm"
        >
          Copy to clipboard
        </button>
        <button
          onClick={onClose}
          className="w-full bg-brand-600 text-white py-2 rounded-lg text-sm font-medium"
        >
          Done
        </button>
      </div>
    </div>
  );
}

export default function KeysPage() {
  const [showCreate, setShowCreate] = useState(false);
  const [newKey, setNewKey] = useState<string | null>(null);
  const [page, setPage] = useState(1);
  const [limit, setLimit] = useState(50);
  const qc = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ["keys", page, limit],
    queryFn: () => keys.list({ page, limit }),
    refetchInterval: 30_000,
  });

  const keyList = data?.data ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / limit));

  const deactivateMutation = useMutation({
    mutationFn: (id: string) => keys.deactivate(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["keys"] }),
  });

  const regenMutation = useMutation({
    mutationFn: (id: string) => keys.regenerate(id),
    onSuccess: (data) => {
      qc.invalidateQueries({ queryKey: ["keys"] });
      setNewKey(data.key);
    },
  });

  function handleLimitChange(newLimit: number) {
    setLimit(newLimit);
    setPage(1);
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Virtual Keys</h1>
          <p className="text-sm text-slate-500 mt-1">{total.toLocaleString()} keys total</p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="bg-brand-600 text-white px-4 py-2 rounded-lg text-sm font-medium hover:bg-brand-700"
        >
          + New Key
        </button>
      </div>

      {isLoading ? (
        <p className="text-sm text-slate-400">Loading…</p>
      ) : (
        <div className="bg-white rounded-xl border border-slate-200 shadow-sm overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 border-b border-slate-200">
              <tr>
                {["Name", "Prefix", "Status", "RPM", "TPM", "Budget", "Last Used", "Actions"].map(
                  (col) => (
                    <th key={col} className="text-left px-4 py-3 font-medium text-slate-600 text-xs uppercase tracking-wide">
                      {col}
                    </th>
                  )
                )}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {keyList.map((k: VirtualKey) => (
                <tr key={k.id} className="hover:bg-slate-50">
                  <td className="px-4 py-3 font-medium">{k.name || "—"}</td>
                  <td className="px-4 py-3 font-mono text-xs text-slate-500">{k.key_prefix}</td>
                  <td className="px-4 py-3">
                    <Badge active={k.is_active} />
                  </td>
                  <td className="px-4 py-3 text-slate-500">{k.rpm_limit ?? "∞"}</td>
                  <td className="px-4 py-3 text-slate-500">{k.tpm_limit ?? "∞"}</td>
                  <td className="px-4 py-3 text-slate-500">
                    {k.budget_usd != null ? `$${k.budget_usd}` : "∞"}
                  </td>
                  <td className="px-4 py-3 text-slate-400 text-xs">
                    {k.last_used_at ? new Date(k.last_used_at).toLocaleDateString() : "Never"}
                  </td>
                  <td className="px-4 py-3">
                    <div className="flex gap-2">
                      <button
                        onClick={() => regenMutation.mutate(k.id)}
                        className="text-xs text-brand-600 hover:underline"
                      >
                        Rotate
                      </button>
                      {k.is_active && (
                        <button
                          onClick={() => deactivateMutation.mutate(k.id)}
                          className="text-xs text-red-500 hover:underline"
                        >
                          Disable
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          {keyList.length === 0 && (
            <p className="text-center text-slate-400 py-8 text-sm">No keys yet.</p>
          )}
        </div>
      )}

      {/* Pagination footer */}
      {total > 0 && (
        <div className="flex items-center justify-between text-sm text-slate-500">
          <span>{total.toLocaleString()} total keys</span>
          <div className="flex items-center gap-4">
            <div className="flex items-center gap-1">
              <button onClick={() => setPage(1)} disabled={page === 1} className="px-2 py-1.5 rounded-lg border border-slate-300 hover:bg-slate-50 disabled:opacity-40 disabled:cursor-not-allowed" aria-label="First page">«</button>
              <button onClick={() => setPage(page - 1)} disabled={page === 1} className="px-2 py-1.5 rounded-lg border border-slate-300 hover:bg-slate-50 disabled:opacity-40 disabled:cursor-not-allowed" aria-label="Previous page">‹</button>
              <span className="px-3 py-1.5">{page} / {totalPages}</span>
              <button onClick={() => setPage(page + 1)} disabled={page >= totalPages} className="px-2 py-1.5 rounded-lg border border-slate-300 hover:bg-slate-50 disabled:opacity-40 disabled:cursor-not-allowed" aria-label="Next page">›</button>
              <button onClick={() => setPage(totalPages)} disabled={page >= totalPages} className="px-2 py-1.5 rounded-lg border border-slate-300 hover:bg-slate-50 disabled:opacity-40 disabled:cursor-not-allowed" aria-label="Last page">»</button>
            </div>
            <div className="flex items-center gap-2">
              <span>Per page:</span>
              <select
                value={limit}
                onChange={(e) => handleLimitChange(Number(e.target.value))}
                className="border border-slate-300 rounded-lg px-3 py-1.5 text-sm"
              >
                {[25, 50, 100, 200].map((n) => (
                  <option key={n} value={n}>{n}</option>
                ))}
              </select>
            </div>
          </div>
        </div>
      )}

      {showCreate && (
        <CreateKeyDialog
          onClose={() => setShowCreate(false)}
          onCreated={(key) => {
            setShowCreate(false);
            setNewKey(key);
          }}
        />
      )}
      {newKey && <NewKeyDisplay rawKey={newKey} onClose={() => setNewKey(null)} />}
    </div>
  );
}

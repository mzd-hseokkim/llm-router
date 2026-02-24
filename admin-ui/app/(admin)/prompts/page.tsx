"use client";

import React, { useState, useRef, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { prompts, Prompt, PromptVersion } from "@/lib/api";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function escapeHtml(text: string): string {
  return text
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function highlightVariables(text: string): string {
  const escaped = escapeHtml(text);
  // Empty variable {{}} → red warning
  const withEmpty = escaped.replace(
    /\{\{\}\}/g,
    '<mark class="bg-red-200 text-red-800 rounded px-0.5">{{}}</mark>'
  );
  // Named variables {{name}} → amber highlight
  return withEmpty.replace(
    /\{\{([^}]+)\}\}/g,
    (match) => `<mark class="bg-amber-200 text-amber-800 rounded px-0.5">${match}</mark>`
  );
}

function suggestNextVersion(versionStrings: string[]): string {
  if (versionStrings.length === 0) return "";
  const last = versionStrings[versionStrings.length - 1];
  const match = last.match(/^(.*?)(\d+)([^0-9]*)$/);
  return match ? `${match[1]}${parseInt(match[2]) + 1}${match[3]}` : "";
}

type DiffLine = { line: string; type: "removed" | "added" | "unchanged" };

function computeDiff(fromText: string, toText: string): DiffLine[] {
  const fromLines = fromText.split("\n");
  const toLines = toText.split("\n");
  const m = fromLines.length;
  const n = toLines.length;

  // LCS DP table
  const dp: number[][] = Array.from({ length: m + 1 }, () =>
    new Array(n + 1).fill(0)
  );
  for (let i = 1; i <= m; i++) {
    for (let j = 1; j <= n; j++) {
      dp[i][j] =
        fromLines[i - 1] === toLines[j - 1]
          ? dp[i - 1][j - 1] + 1
          : Math.max(dp[i - 1][j], dp[i][j - 1]);
    }
  }

  // Backtrack to produce diff
  const result: DiffLine[] = [];
  let i = m,
    j = n;
  while (i > 0 || j > 0) {
    if (i > 0 && j > 0 && fromLines[i - 1] === toLines[j - 1]) {
      result.unshift({ line: fromLines[i - 1], type: "unchanged" });
      i--;
      j--;
    } else if (j > 0 && (i === 0 || dp[i][j - 1] >= dp[i - 1][j])) {
      result.unshift({ line: toLines[j - 1], type: "added" });
      j--;
    } else {
      result.unshift({ line: fromLines[i - 1], type: "removed" });
      i--;
    }
  }
  return result;
}

// ---------------------------------------------------------------------------
// VariableEditor
// ---------------------------------------------------------------------------

function VariableEditor({
  value,
  onChange,
  placeholder,
  rows = 6,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  rows?: number;
}) {
  const overlayRef = useRef<HTMLDivElement>(null);

  function handleScroll(e: React.UIEvent<HTMLTextAreaElement>) {
    if (overlayRef.current) {
      overlayRef.current.scrollTop = e.currentTarget.scrollTop;
      overlayRef.current.scrollLeft = e.currentTarget.scrollLeft;
    }
  }

  const sharedStyle: React.CSSProperties = {
    fontFamily:
      'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Courier New", monospace',
    fontSize: "13px",
    lineHeight: "1.6",
    padding: "8px 12px",
    whiteSpace: "pre-wrap",
    wordBreak: "break-word",
    overflowWrap: "break-word",
  };

  // Calculate height from rows (13px * 1.6 line-height + padding)
  const minHeight = `${rows * 13 * 1.6 + 16}px`;

  return (
    <div
      className="relative border border-slate-300 rounded-lg overflow-hidden focus-within:ring-2 focus-within:ring-brand-500"
      style={{ minHeight, maxHeight: "320px" }}
    >
      {/* Overlay — renders highlighted HTML, syncs scroll position */}
      <div
        ref={overlayRef}
        aria-hidden
        className="absolute inset-0 pointer-events-none"
        style={{ ...sharedStyle, overflow: "hidden" }}
        dangerouslySetInnerHTML={{ __html: highlightVariables(value) }}
      />
      {/* Textarea — transparent text so overlay shows through, visible caret */}
      <textarea
        className="absolute inset-0 w-full h-full bg-transparent resize-none focus:outline-none"
        style={{
          ...sharedStyle,
          color: "transparent",
          caretColor: "#334155",
          overflowY: "auto",
        }}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        onScroll={handleScroll}
        placeholder={placeholder}
        spellCheck={false}
      />
    </div>
  );
}

// ---------------------------------------------------------------------------
// DiffView
// ---------------------------------------------------------------------------

function DiffView({
  fromTemplate,
  toTemplate,
  fromVersion,
  toVersion,
}: {
  fromTemplate: string;
  toTemplate: string;
  fromVersion: string;
  toVersion: string;
}) {
  const lines = computeDiff(fromTemplate, toTemplate);
  return (
    <div className="font-mono text-xs border border-slate-200 rounded overflow-hidden">
      <div className="flex gap-4 bg-slate-50 px-3 py-1.5 border-b border-slate-200">
        <span className="text-red-600">— {fromVersion}</span>
        <span className="text-green-600">+ {toVersion}</span>
      </div>
      <div className="overflow-auto max-h-56">
        {lines.map((l, idx) => (
          <div
            key={idx}
            className={`flex gap-2 px-3 py-0.5 ${
              l.type === "removed"
                ? "bg-red-50 text-red-700"
                : l.type === "added"
                ? "bg-green-50 text-green-700"
                : "text-slate-600"
            }`}
          >
            <span className="w-4 flex-shrink-0 text-slate-400 select-none text-right">
              {l.type === "removed" ? "−" : l.type === "added" ? "+" : " "}
            </span>
            <span className={l.type === "removed" ? "line-through" : ""}>
              {l.line || "\u00A0"}
            </span>
          </div>
        ))}
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// VersionPanel
// ---------------------------------------------------------------------------

function VersionPanel({
  prompt,
  onClose,
}: {
  prompt: Prompt;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [diffFrom, setDiffFrom] = useState("");
  const [diffTo, setDiffTo] = useState("");
  const [diffData, setDiffData] = useState<{
    from: { version: string; template: string };
    to: { version: string; template: string };
  } | null>(null);
  const [diffError, setDiffError] = useState("");
  const [rollbackError, setRollbackError] = useState("");

  const { data: versions = [], isLoading } = useQuery({
    queryKey: ["prompt-versions", prompt.slug],
    queryFn: () => prompts.versions.list(prompt.slug),
  });

  const rollbackMutation = useMutation({
    mutationFn: (version: string) => prompts.rollback(prompt.slug, version),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["prompts"] });
      qc.invalidateQueries({ queryKey: ["prompt-versions", prompt.slug] });
      setRollbackError("");
    },
    onError: (e: Error) => setRollbackError(e.message),
  });

  const diffMutation = useMutation({
    mutationFn: ({ from, to }: { from: string; to: string }) =>
      prompts.diff(prompt.slug, from, to),
    onSuccess: (data) => {
      setDiffData(data);
      setDiffError("");
    },
    onError: (e: Error) => {
      setDiffError(e.message);
      setDiffData(null);
    },
  });

  const canCompare = !!diffFrom && !!diffTo && diffFrom !== diffTo;

  return (
    <div className="bg-slate-50 border border-slate-200 rounded-lg p-4 space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-slate-700">
          Version History —{" "}
          <span className="font-normal text-slate-500">{prompt.name}</span>
        </h3>
        <button
          onClick={onClose}
          className="text-xs text-slate-400 hover:text-slate-700"
        >
          ✕ Close
        </button>
      </div>

      {isLoading ? (
        <p className="text-xs text-slate-400">Loading versions…</p>
      ) : (
        <>
          {rollbackError && (
            <p className="text-xs text-red-600">{rollbackError}</p>
          )}
          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-slate-200 text-left text-slate-500">
                  <th className="px-3 py-2 font-medium">Version</th>
                  <th className="px-3 py-2 font-medium">Status</th>
                  <th className="px-3 py-2 font-medium">Created At</th>
                  <th className="px-3 py-2 font-medium"></th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {versions.map((v: PromptVersion) => (
                  <tr key={v.id} className="hover:bg-slate-100">
                    <td className="px-3 py-2 font-mono">{v.version}</td>
                    <td className="px-3 py-2">
                      {v.is_active ? (
                        <span className="inline-flex items-center gap-1 bg-green-100 text-green-700 px-2 py-0.5 rounded text-xs font-medium">
                          ✓ Active
                        </span>
                      ) : (
                        <span className="text-slate-400">—</span>
                      )}
                    </td>
                    <td className="px-3 py-2 text-slate-400">
                      {new Date(v.created_at).toLocaleString()}
                    </td>
                    <td className="px-3 py-2">
                      <button
                        disabled={!!v.is_active || rollbackMutation.isPending}
                        onClick={() => rollbackMutation.mutate(v.version)}
                        className="text-xs text-brand-600 hover:underline disabled:opacity-40 disabled:cursor-not-allowed"
                      >
                        Rollback
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {versions.length === 0 && (
              <p className="text-center text-slate-400 py-4 text-xs">
                No versions yet.
              </p>
            )}
          </div>

          {versions.length >= 2 && (
            <div className="border-t border-slate-200 pt-4 space-y-3">
              <h4 className="text-xs font-semibold text-slate-600">
                Compare Versions
              </h4>
              <div className="flex items-center gap-3 flex-wrap">
                <select
                  className="border border-slate-300 rounded px-2 py-1 text-xs bg-white"
                  value={diffFrom}
                  onChange={(e) => setDiffFrom(e.target.value)}
                >
                  <option value="">From…</option>
                  {versions.map((v: PromptVersion) => (
                    <option key={v.id} value={v.version}>
                      {v.version}
                    </option>
                  ))}
                </select>
                <select
                  className="border border-slate-300 rounded px-2 py-1 text-xs bg-white"
                  value={diffTo}
                  onChange={(e) => setDiffTo(e.target.value)}
                >
                  <option value="">To…</option>
                  {versions.map((v: PromptVersion) => (
                    <option key={v.id} value={v.version}>
                      {v.version}
                    </option>
                  ))}
                </select>
                <button
                  disabled={!canCompare || diffMutation.isPending}
                  onClick={() =>
                    diffMutation.mutate({ from: diffFrom, to: diffTo })
                  }
                  className="text-xs bg-slate-700 text-white px-3 py-1 rounded disabled:opacity-40 disabled:cursor-not-allowed"
                  title={!canCompare ? "최소 2개 버전 필요" : undefined}
                >
                  {diffMutation.isPending ? "Loading…" : "Compare"}
                </button>
              </div>
              {diffError && (
                <p className="text-xs text-red-600">
                  Diff 로드 실패: {diffError}
                </p>
              )}
              {diffData && (
                <DiffView
                  fromTemplate={diffData.from.template}
                  toTemplate={diffData.to.template}
                  fromVersion={diffData.from.version}
                  toVersion={diffData.to.version}
                />
              )}
            </div>
          )}
          {versions.length === 1 && (
            <p className="text-xs text-slate-400 border-t border-slate-200 pt-3">
              최소 2개 버전이 필요합니다.
            </p>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// CreatePromptDialog
// ---------------------------------------------------------------------------

function CreatePromptDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: () => void;
}) {
  const qc = useQueryClient();
  const [form, setForm] = useState({
    slug: "",
    name: "",
    template: "",
    team_id: "",
  });
  const [error, setError] = useState("");

  const mutation = useMutation({
    mutationFn: () =>
      prompts.create({
        slug: form.slug,
        name: form.name,
        template: form.template,
        ...(form.team_id ? { team_id: form.team_id } : {}),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["prompts"] });
      onCreated();
    },
    onError: (e: Error) => setError(e.message),
  });

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-lg p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <h2 className="text-lg font-semibold text-slate-900">New Prompt</h2>

        {error && (
          <p className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</p>
        )}

        <div className="space-y-3">
          <label className="block">
            <span className="text-xs font-medium text-slate-600">
              Slug * (lowercase, hyphens)
            </span>
            <input
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm font-mono"
              placeholder="my-prompt"
              value={form.slug}
              onChange={(e) => setForm((f) => ({ ...f, slug: e.target.value }))}
            />
          </label>

          <label className="block">
            <span className="text-xs font-medium text-slate-600">Name *</span>
            <input
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
            />
          </label>

          <div>
            <span className="text-xs font-medium text-slate-600">
              Template *
            </span>
            <div className="mt-1">
              <VariableEditor
                value={form.template}
                onChange={(v) => setForm((f) => ({ ...f, template: v }))}
                placeholder="Enter template text with {{variables}}"
              />
            </div>
          </div>

          <label className="block">
            <span className="text-xs font-medium text-slate-600">
              Team ID (optional)
            </span>
            <input
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm font-mono"
              value={form.team_id}
              onChange={(e) =>
                setForm((f) => ({ ...f, team_id: e.target.value }))
              }
            />
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
            disabled={
              !form.slug || !form.name || !form.template || mutation.isPending
            }
            onClick={() => mutation.mutate()}
            className="px-4 py-2 text-sm bg-slate-900 text-white rounded-lg disabled:opacity-50"
          >
            {mutation.isPending ? "Creating…" : "Create"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// EditPromptDialog
// ---------------------------------------------------------------------------

function EditPromptDialog({
  prompt,
  currentVersion,
  onClose,
  onUpdated,
}: {
  prompt: Prompt;
  currentVersion: PromptVersion | null;
  onClose: () => void;
  onUpdated: () => void;
}) {
  const qc = useQueryClient();
  const initRef = useRef(false);
  const [form, setForm] = useState({
    version: "",
    template: currentVersion?.template ?? prompt.template,
  });
  const [error, setError] = useState("");

  const { data: versions = [] } = useQuery({
    queryKey: ["prompt-versions", prompt.slug],
    queryFn: () => prompts.versions.list(prompt.slug),
  });

  // W-2: auto-suggest next version; W-3: init template from active version
  useEffect(() => {
    if (!initRef.current && versions.length > 0) {
      initRef.current = true;
      const active = versions.find((v) => v.is_active);
      const suggested = suggestNextVersion(versions.map((v) => v.version));
      setForm((f) => ({
        ...f,
        template: active ? active.template : f.template,
        ...(suggested ? { version: suggested } : {}),
      }));
    }
  }, [versions]);

  const mutation = useMutation({
    mutationFn: () => prompts.versions.create(prompt.slug, form),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["prompts"] });
      qc.invalidateQueries({ queryKey: ["prompt-versions", prompt.slug] });
      onUpdated();
    },
    onError: (e: Error) => setError(e.message),
  });

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-lg p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <h2 className="text-lg font-semibold text-slate-900">
          Edit Prompt —{" "}
          <span className="font-normal text-slate-500">{prompt.name}</span>
        </h2>

        {versions.length > 0 && (
          <p className="text-xs text-slate-400">
            Existing versions: {versions.map((v) => v.version).join(", ")}
          </p>
        )}

        {error && (
          <p className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</p>
        )}

        <div className="space-y-3">
          <label className="block">
            <span className="text-xs font-medium text-slate-600">
              New Version *
            </span>
            <input
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm font-mono"
              placeholder="e.g. 1.0.1, v2, …"
              value={form.version}
              onChange={(e) =>
                setForm((f) => ({ ...f, version: e.target.value }))
              }
            />
          </label>

          <div>
            <span className="text-xs font-medium text-slate-600">
              Template *
            </span>
            <div className="mt-1">
              <VariableEditor
                value={form.template}
                onChange={(v) => setForm((f) => ({ ...f, template: v }))}
              />
            </div>
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
            disabled={!form.version || !form.template || mutation.isPending}
            onClick={() => mutation.mutate()}
            className="px-4 py-2 text-sm bg-slate-900 text-white rounded-lg disabled:opacity-50"
          >
            {mutation.isPending ? "Saving…" : "Save Version"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// PromptsPage
// ---------------------------------------------------------------------------

export default function PromptsPage() {
  const [showCreate, setShowCreate] = useState(false);
  const [editPrompt, setEditPrompt] = useState<Prompt | null>(null);
  const [versionPrompt, setVersionPrompt] = useState<Prompt | null>(null);
  const [page, setPage] = useState(1);
  const [limit, setLimit] = useState(50);

  const { data, isLoading } = useQuery({
    queryKey: ["prompts", page, limit],
    queryFn: () => prompts.list({ page, limit }),
    refetchInterval: 30_000,
  });

  const promptList = data?.data ?? [];
  const total = data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / limit));

  function handleVersionToggle(p: Prompt) {
    setVersionPrompt((prev) => (prev?.id === p.id ? null : p));
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Prompts</h1>
          <p className="text-sm text-slate-500 mt-1">
            {total.toLocaleString()} prompts total
          </p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="bg-brand-600 text-white px-4 py-2 rounded-lg text-sm font-medium hover:bg-brand-700"
        >
          + New Prompt
        </button>
      </div>

      {isLoading ? (
        <p className="text-sm text-slate-400">Loading…</p>
      ) : promptList.length === 0 ? (
        <div className="bg-white rounded-xl border border-slate-200 p-8 text-center space-y-3">
          <p className="text-sm text-slate-400">No prompts yet.</p>
          <button
            onClick={() => setShowCreate(true)}
            className="text-sm text-brand-600 hover:underline"
          >
            Create your first prompt
          </button>
        </div>
      ) : (
        <div className="bg-white rounded-xl border border-slate-200 shadow-sm overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 border-b border-slate-200">
              <tr>
                {["Name", "Slug", "Updated At", "Actions"].map((col) => (
                  <th
                    key={col}
                    className="text-left px-4 py-3 font-medium text-slate-600 text-xs uppercase tracking-wide"
                  >
                    {col}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {promptList.map((p: Prompt) => (
                <React.Fragment key={p.id}>
                  <tr className="hover:bg-slate-50">
                    <td className="px-4 py-3 font-medium">{p.name}</td>
                    <td className="px-4 py-3 font-mono text-xs text-slate-500">
                      {p.slug}
                    </td>
                    <td className="px-4 py-3 text-slate-400 text-xs">
                      {new Date(p.updated_at).toLocaleString()}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex gap-2">
                        <button
                          onClick={() => setEditPrompt(p)}
                          className="text-xs text-brand-600 hover:underline"
                        >
                          Edit
                        </button>
                        <button
                          onClick={() => handleVersionToggle(p)}
                          className="text-xs text-slate-500 hover:underline"
                        >
                          {versionPrompt?.id === p.id
                            ? "Hide Versions"
                            : "Versions"}
                        </button>
                      </div>
                    </td>
                  </tr>
                  {versionPrompt?.id === p.id && (
                    <tr>
                      <td colSpan={4} className="px-4 py-2">
                        <VersionPanel
                          prompt={p}
                          onClose={() => setVersionPrompt(null)}
                        />
                      </td>
                    </tr>
                  )}
                </React.Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {/* Pagination footer */}
      {total > 0 && (
        <div className="flex items-center justify-between text-sm text-slate-500">
          <span>{total.toLocaleString()} total prompts</span>
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
                onChange={(e) => { setLimit(Number(e.target.value)); setPage(1); }}
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
        <CreatePromptDialog
          onClose={() => setShowCreate(false)}
          onCreated={() => setShowCreate(false)}
        />
      )}
      {editPrompt && (
        <EditPromptDialog
          prompt={editPrompt}
          currentVersion={null}
          onClose={() => setEditPrompt(null)}
          onUpdated={() => setEditPrompt(null)}
        />
      )}
    </div>
  );
}

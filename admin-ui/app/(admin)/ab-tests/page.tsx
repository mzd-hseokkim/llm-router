"use client";

import React, { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { abTests, models as modelsApi, ABTest, Model } from "@/lib/api";

// ---------------------------------------------------------------------------
// Local types — supplement lib/api.ts which lacks some backend fields
// ---------------------------------------------------------------------------

interface TrafficSplitWithModel {
  variant: string;
  model: string;
  weight: number;
}

interface VariantResult {
  model: string;
  samples: number;
  latency_p95_ms: number;
  avg_cost_per_request: number;
  error_rate: number;
}

interface MetricSignificance {
  p_value: number;
  significant: boolean;
  improvement_pct: number;
}

interface AnalysisResult {
  test_id: string;
  status: string;
  winner?: string;
  results: Record<string, VariantResult>;
  statistical_significance?: Record<string, MetricSignificance>;
  recommendation?: string;
}

interface TrafficSplitFormRow {
  variant: string;
  model: string;
  weight: number;
}

interface CreateFormState {
  name: string;
  splits: TrafficSplitFormRow[];
  sample_rate: number;
  min_samples: number;
  confidence_level: number;
  start_at: string;
  end_at: string;
}

// ---------------------------------------------------------------------------
// StatusBadge
// ---------------------------------------------------------------------------

const STATUS_COLORS: Record<string, string> = {
  draft:     "bg-slate-100 text-slate-600",
  running:   "bg-green-100 text-green-700",
  paused:    "bg-yellow-100 text-yellow-700",
  completed: "bg-blue-100 text-blue-700",
  stopped:   "bg-red-100 text-red-700",
};

function StatusBadge({ status }: { status: string }) {
  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium
                  ${STATUS_COLORS[status] ?? "bg-slate-100 text-slate-600"}`}
    >
      {status}
    </span>
  );
}

// ---------------------------------------------------------------------------
// CreateDialog
// ---------------------------------------------------------------------------

function CreateDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: () => void;
}) {
  const qc = useQueryClient();
  const { data: availableModels = [] } = useQuery<Model[]>({
    queryKey: ["models-all"],
    queryFn: modelsApi.listAll,
  });
  const [form, setForm] = useState<CreateFormState>({
    name: "",
    splits: [
      { variant: "control",   model: "", weight: 50 },
      { variant: "treatment", model: "", weight: 50 },
    ],
    sample_rate: 1.0,
    min_samples: 1000,
    confidence_level: 0.95,
    start_at: "",
    end_at: "",
  });
  const [error, setError] = useState("");

  const totalWeight = form.splits.reduce((sum, s) => sum + (Number(s.weight) || 0), 0);
  const weightValid = totalWeight === 100;

  const mutation = useMutation({
    mutationFn: () =>
      abTests.create({
        name: form.name.trim(),
        traffic_split: form.splits.map(s => ({ variant: s.variant, model: s.model, weight: s.weight })),
        target: { sample_rate: form.sample_rate },
        min_samples: form.min_samples,
        confidence_level: form.confidence_level,
        ...(form.start_at ? { start_at: new Date(form.start_at).toISOString() } : {}),
        ...(form.end_at   ? { end_at:   new Date(form.end_at).toISOString()   } : {}),
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["ab-tests"] });
      onCreated();
    },
    onError: (e: Error) => setError(e.message),
  });

  const canSubmit =
    form.name.trim().length > 0 &&
    form.splits.length >= 2 &&
    form.splits.every(s => s.variant.trim().length > 0 && s.model.trim().length > 0) &&
    weightValid &&
    !mutation.isPending;

  function addSplit() {
    setForm(f => ({
      ...f,
      splits: [...f.splits, { variant: "", model: "", weight: 0 }],
    }));
  }

  function removeSplit(idx: number) {
    setForm(f => ({
      ...f,
      splits: f.splits.filter((_, i) => i !== idx),
    }));
  }

  function updateSplit(idx: number, field: keyof TrafficSplitFormRow, value: string | number) {
    setForm(f => ({
      ...f,
      splits: f.splits.map((s, i) => i === idx ? { ...s, [field]: value } : s),
    }));
  }

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-lg p-6 space-y-4 max-h-[90vh] overflow-y-auto">
        <h2 className="text-lg font-semibold text-slate-900">New A/B Test</h2>

        {error && (
          <p className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</p>
        )}

        <div className="space-y-4">
          {/* Name */}
          <label className="block">
            <span className="text-xs font-medium text-slate-600">Experiment Name *</span>
            <input
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              placeholder="Experiment name"
              value={form.name}
              onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
            />
          </label>

          {/* Traffic Split */}
          <div>
            <p className="text-xs font-medium text-slate-600 mb-2">Traffic Split</p>
            <div className="space-y-2">
              {form.splits.map((s, idx) => (
                <div key={idx} className="flex gap-2 items-center">
                  <input
                    className="flex-1 border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                    placeholder="variant"
                    value={s.variant}
                    onChange={e => updateSplit(idx, "variant", e.target.value)}
                  />
                  <select
                    className="flex-1 border border-slate-300 rounded-lg px-2 py-1.5 text-sm bg-white"
                    value={s.model}
                    onChange={e => updateSplit(idx, "model", e.target.value)}
                  >
                    <option value="">— Select model —</option>
                    {availableModels.map(m => (
                      <option key={m.id} value={m.model_id}>
                        {m.display_name ?? m.model_id}
                      </option>
                    ))}
                  </select>
                  <input
                    className="w-20 border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                    type="number"
                    min={0}
                    max={100}
                    value={s.weight}
                    onChange={e => {
                      const parsed = Number(e.target.value);
                      updateSplit(idx, "weight", isNaN(parsed) ? 0 : parsed);
                    }}
                  />
                  <button
                    onClick={() => removeSplit(idx)}
                    disabled={form.splits.length <= 2}
                    className="text-slate-400 hover:text-red-500 disabled:opacity-30 text-sm px-1"
                    title="Remove"
                  >
                    ×
                  </button>
                </div>
              ))}
            </div>
            <button
              onClick={addSplit}
              className="mt-2 text-xs text-brand-600 hover:underline"
            >
              + Add Variant
            </button>
            {!weightValid && form.splits.length >= 2 && (
              <p className="text-xs text-red-500 mt-1">
                Weights must sum to 100 — current total: {totalWeight}
              </p>
            )}
          </div>

          {/* Options */}
          <div className="grid grid-cols-3 gap-2">
            <label className="block">
              <span className="text-xs font-medium text-slate-600">Min Samples</span>
              <input
                type="number"
                className="mt-1 block w-full border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                value={form.min_samples}
                onChange={e => setForm(f => ({ ...f, min_samples: Number(e.target.value) }))}
              />
            </label>
            <label className="block">
              <span className="text-xs font-medium text-slate-600">Confidence</span>
              <input
                type="number"
                step="0.01"
                min="0"
                max="1"
                className="mt-1 block w-full border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                value={form.confidence_level}
                onChange={e => setForm(f => ({ ...f, confidence_level: Number(e.target.value) }))}
              />
            </label>
            <label className="block">
              <span className="text-xs font-medium text-slate-600">Sample Rate</span>
              <input
                type="number"
                step="0.1"
                min="0"
                max="1"
                className="mt-1 block w-full border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                value={form.sample_rate}
                onChange={e => setForm(f => ({ ...f, sample_rate: Number(e.target.value) }))}
              />
            </label>
          </div>

          <div className="grid grid-cols-2 gap-2">
            <label className="block">
              <span className="text-xs font-medium text-slate-600">Start At (optional)</span>
              <input
                type="datetime-local"
                className="mt-1 block w-full border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                value={form.start_at}
                onChange={e => setForm(f => ({ ...f, start_at: e.target.value }))}
              />
            </label>
            <label className="block">
              <span className="text-xs font-medium text-slate-600">End At (optional)</span>
              <input
                type="datetime-local"
                className="mt-1 block w-full border border-slate-300 rounded-lg px-2 py-1.5 text-sm"
                value={form.end_at}
                onChange={e => setForm(f => ({ ...f, end_at: e.target.value }))}
              />
            </label>
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
            className="px-4 py-2 text-sm bg-slate-900 text-white rounded-lg disabled:opacity-50"
          >
            {mutation.isPending ? "Creating…" : "Create Experiment"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// ResultsPanel
// ---------------------------------------------------------------------------

function ResultsPanel({ test, onClose }: { test: ABTest; onClose: () => void }) {
  const { data: rawResults, isLoading, error } = useQuery({
    queryKey: ["ab-test-results", test.id],
    queryFn:  () => abTests.results(test.id),
    staleTime: 10_000,
  });

  const analysis = rawResults as unknown as AnalysisResult | undefined;
  const variantEntries = analysis?.results ? Object.entries(analysis.results) : [];

  return (
    <div className="bg-slate-50 border border-slate-200 rounded-lg p-4 space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-semibold text-slate-700">
          Results — <span className="font-normal text-slate-500">{test.name}</span>
        </h3>
        <button onClick={onClose} className="text-xs text-slate-400 hover:text-slate-700">
          ✕ Close
        </button>
      </div>

      {isLoading ? (
        <p className="text-sm text-slate-400">Loading results…</p>
      ) : error ? (
        <p className="text-sm text-red-600">
          결과를 불러오지 못했습니다: {(error as Error).message}
        </p>
      ) : !analysis?.results || Object.keys(analysis.results).length === 0 ? (
        <p className="text-sm text-slate-400">No results yet (experiment has not started or has 0 samples)</p>
      ) : (
        <>
          {analysis.winner && (
            <p className="text-xs text-green-700 font-medium">Winner: {analysis.winner}</p>
          )}

          <div className="overflow-x-auto">
            <table className="w-full text-xs">
              <thead>
                <tr className="border-b border-slate-200 text-left text-slate-500">
                  {["Variant", "Model", "Samples", "Latency P95 (ms)", "Avg Cost/Req ($)", "Error Rate (%)"].map(col => (
                    <th key={col} className="px-3 py-2 font-medium">{col}</th>
                  ))}
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {variantEntries.map(([variant, v]) => (
                  <tr key={variant} className="hover:bg-slate-100">
                    <td className="px-3 py-2 font-medium">{variant}</td>
                    <td className="px-3 py-2 text-slate-500">{v.model}</td>
                    <td className="px-3 py-2">
                      {v.samples === 0 ? (
                        <em className="text-slate-400">No data yet</em>
                      ) : (
                        v.samples.toLocaleString()
                      )}
                    </td>
                    <td className="px-3 py-2">{v.latency_p95_ms.toFixed(1)}</td>
                    <td className="px-3 py-2">${v.avg_cost_per_request.toFixed(4)}</td>
                    <td className="px-3 py-2">{(v.error_rate * 100).toFixed(2)}%</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          {analysis.statistical_significance && Object.keys(analysis.statistical_significance).length > 0 && (
            <div>
              <h4 className="text-xs font-semibold text-slate-600 mb-2">Statistical Significance</h4>
              <table className="w-full text-xs">
                <thead>
                  <tr className="border-b border-slate-200 text-left text-slate-500">
                    {["Metric", "P-Value", "Significant", "Improvement"].map(col => (
                      <th key={col} className="px-3 py-2 font-medium">{col}</th>
                    ))}
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100">
                  {Object.entries(analysis.statistical_significance).map(([metric, sig]) => (
                    <tr key={metric}>
                      <td className="px-3 py-2 font-mono">{metric}</td>
                      <td className="px-3 py-2">{sig.p_value.toFixed(3)}</td>
                      <td className="px-3 py-2">
                        {sig.significant ? (
                          <span className="text-green-700 font-medium">Yes</span>
                        ) : (
                          <span className="text-slate-400">No</span>
                        )}
                      </td>
                      <td className="px-3 py-2">{sig.improvement_pct.toFixed(1)}%</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          {analysis.recommendation && (
            <div className="p-3 bg-blue-50 rounded text-sm text-blue-800">
              <span className="font-medium">Recommendation: </span>
              {analysis.recommendation}
            </div>
          )}
        </>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// PromoteDialog
// ---------------------------------------------------------------------------

function PromoteDialog({ test, onClose }: { test: ABTest; onClose: () => void }) {
  const qc = useQueryClient();
  const existingWinner = (test as unknown as { winner?: string }).winner ?? "";
  const [winner, setWinner] = useState<string>(existingWinner);
  const [error, setError] = useState("");

  const mutation = useMutation({
    mutationFn: () => abTests.promote(test.id, winner),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["ab-tests"] });
      onClose();
    },
    onError: (e: Error) => setError(e.message),
  });

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-sm p-6 space-y-4">
        <h2 className="text-lg font-semibold text-slate-900">Promote Winner</h2>
        <p className="text-sm text-slate-500">
          Select the winning variant for <strong>{test.name}</strong>.
          The experiment will be marked as completed.
        </p>

        {error && <p className="text-sm text-red-600 bg-red-50 rounded p-2">{error}</p>}

        <div>
          <label className="text-xs font-medium text-slate-600">Winner Variant</label>
          <select
            value={winner}
            onChange={e => setWinner(e.target.value)}
            className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
          >
            <option value="">— Select variant —</option>
            {(test.traffic_split as unknown as TrafficSplitWithModel[]).map(s => (
              <option key={s.variant} value={s.variant}>{s.variant}</option>
            ))}
          </select>
        </div>

        <div className="flex justify-end gap-2 pt-2">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm text-slate-600 hover:bg-slate-100 rounded-lg"
          >
            Cancel
          </button>
          <button
            disabled={!winner || mutation.isPending}
            onClick={() => mutation.mutate()}
            className="px-4 py-2 text-sm bg-slate-900 text-white rounded-lg disabled:opacity-50"
          >
            {mutation.isPending ? "Promoting…" : "Promote"}
          </button>
        </div>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// ABTestsPage
// ---------------------------------------------------------------------------

export default function ABTestsPage() {
  const qc = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [resultsTest, setResultsTest] = useState<ABTest | null>(null);
  const [promoteTest, setPromoteTest] = useState<ABTest | null>(null);
  const [transitionError, setTransitionError] = useState<string | null>(null);

  const { data: testList = [], isLoading, error: listError } = useQuery({
    queryKey: ["ab-tests"],
    queryFn: abTests.list,
    refetchInterval: 30_000,
  });

  const startMutation = useMutation({
    mutationFn: (id: string) => abTests.start(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["ab-tests"] }),
    onError: (e: Error) => setTransitionError(e.message),
  });
  const pauseMutation = useMutation({
    mutationFn: (id: string) => abTests.pause(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["ab-tests"] }),
    onError: (e: Error) => setTransitionError(e.message),
  });
  const stopMutation = useMutation({
    mutationFn: (id: string) => abTests.stop(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["ab-tests"] }),
    onError: (e: Error) => setTransitionError(e.message),
  });
  const deleteMutation = useMutation({
    mutationFn: (id: string) => abTests.delete(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["ab-tests"] }),
    onError: (e: Error) => setTransitionError(e.message),
  });

  function handleResultsToggle(test: ABTest) {
    setResultsTest(prev => prev?.id === test.id ? null : test);
  }

  function ActionButtons({ test }: { test: ABTest }) {
    return (
      <div className="flex gap-1.5 flex-wrap">
        {(test.status === "draft" || test.status === "paused") && (
          <button
            onClick={() => { setTransitionError(null); startMutation.mutate(test.id); }}
            className="text-xs px-2 py-1 rounded bg-green-100 text-green-700 hover:bg-green-200"
          >
            Start
          </button>
        )}
        {test.status === "running" && (
          <button
            onClick={() => { setTransitionError(null); pauseMutation.mutate(test.id); }}
            className="text-xs px-2 py-1 rounded bg-yellow-100 text-yellow-700 hover:bg-yellow-200"
          >
            Pause
          </button>
        )}
        {(test.status === "running" || test.status === "paused") && (
          <button
            onClick={() => { setTransitionError(null); stopMutation.mutate(test.id); }}
            className="text-xs px-2 py-1 rounded bg-red-100 text-red-700 hover:bg-red-200"
          >
            Stop
          </button>
        )}
        {(test.status === "running" || test.status === "completed") && (
          <button
            onClick={() => setPromoteTest(test)}
            className="text-xs px-2 py-1 rounded bg-blue-100 text-blue-700 hover:bg-blue-200"
          >
            Promote
          </button>
        )}
        {(test.status === "draft" || test.status === "stopped" || test.status === "completed") && (
          <button
            onClick={() => {
              if (confirm(`"${test.name}" 실험을 삭제하시겠습니까?`)) {
                setTransitionError(null);
                deleteMutation.mutate(test.id);
              }
            }}
            className="text-xs px-2 py-1 rounded bg-red-50 text-red-600 hover:bg-red-100"
          >
            Delete
          </button>
        )}
        {test.status !== "draft" && (
          <button
            onClick={() => handleResultsToggle(test)}
            className="text-xs px-2 py-1 rounded bg-slate-100 text-slate-600 hover:bg-slate-200"
          >
            {resultsTest?.id === test.id ? "Hide Results" : "Results"}
          </button>
        )}
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">A/B Tests</h1>
          <p className="text-sm text-slate-500 mt-1">{testList.length} experiments</p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="bg-brand-600 text-white px-4 py-2 rounded-lg text-sm font-medium hover:bg-brand-700"
        >
          + New A/B Test
        </button>
      </div>

      {transitionError && (
        <p className="text-sm text-red-600 bg-red-50 rounded p-2">{transitionError}</p>
      )}
      {listError && (
        <p className="text-sm text-red-600 bg-red-50 rounded p-2">
          {(listError as Error).message}
        </p>
      )}

      {isLoading ? (
        <p className="text-sm text-slate-400">Loading…</p>
      ) : testList.length === 0 ? (
        <div className="bg-white rounded-xl border border-slate-200 p-8 text-center space-y-3">
          <p className="text-sm text-slate-400">No A/B tests yet.</p>
          <button
            onClick={() => setShowCreate(true)}
            className="text-sm text-brand-600 hover:underline"
          >
            Create your first experiment
          </button>
        </div>
      ) : (
        <div className="bg-white rounded-xl border border-slate-200 shadow-sm overflow-hidden">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 border-b border-slate-200">
              <tr>
                {["Name", "Status", "Traffic Split", "Created At", "Actions"].map(col => (
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
              {testList.map((test: ABTest) => (
                <React.Fragment key={test.id}>
                  <tr className="hover:bg-slate-50">
                    <td className="px-4 py-3 font-medium">{test.name}</td>
                    <td className="px-4 py-3">
                      <StatusBadge status={test.status} />
                    </td>
                    <td className="px-4 py-3 text-xs text-slate-500 font-mono">
                      {(test.traffic_split as unknown as TrafficSplitWithModel[])
                        .map(s => `${s.variant}:${s.weight}%`)
                        .join(" / ")}
                    </td>
                    <td className="px-4 py-3 text-slate-400 text-xs">
                      {(test as unknown as { created_at?: string }).created_at
                        ? new Date((test as unknown as { created_at: string }).created_at).toLocaleString()
                        : "—"}
                    </td>
                    <td className="px-4 py-3">
                      <ActionButtons test={test} />
                    </td>
                  </tr>
                  {resultsTest?.id === test.id && (
                    <tr>
                      <td colSpan={5} className="px-4 py-2">
                        <ResultsPanel test={test} onClose={() => setResultsTest(null)} />
                      </td>
                    </tr>
                  )}
                </React.Fragment>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {showCreate && (
        <CreateDialog
          onClose={() => setShowCreate(false)}
          onCreated={() => setShowCreate(false)}
        />
      )}
      {promoteTest && (
        <PromoteDialog test={promoteTest} onClose={() => setPromoteTest(null)} />
      )}
    </div>
  );
}

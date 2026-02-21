"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { logs, LogEntry } from "@/lib/api";

function StatusBadge({ code }: { code: number }) {
  const cls =
    code >= 500
      ? "bg-red-100 text-red-800"
      : code >= 400
      ? "bg-yellow-100 text-yellow-800"
      : "bg-green-100 text-green-800";
  return <span className={`px-1.5 py-0.5 rounded text-xs font-mono font-medium ${cls}`}>{code}</span>;
}

export default function LogsPage() {
  const [selected, setSelected] = useState<LogEntry | null>(null);
  const [limit, setLimit] = useState(50);

  const { data: logList = [], isLoading, refetch } = useQuery({
    queryKey: ["logs", limit],
    queryFn: () => logs.list({ limit }),
    refetchInterval: 15_000,
  });

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Request Logs</h1>
          <p className="text-sm text-slate-500 mt-1">Last 7 days — auto-refreshes every 15s</p>
        </div>
        <div className="flex items-center gap-3">
          <select
            value={limit}
            onChange={(e) => setLimit(Number(e.target.value))}
            className="border border-slate-300 rounded-lg px-3 py-1.5 text-sm"
          >
            {[25, 50, 100, 200].map((n) => (
              <option key={n} value={n}>
                {n} entries
              </option>
            ))}
          </select>
          <button
            onClick={() => refetch()}
            className="border border-slate-300 text-slate-700 px-3 py-1.5 rounded-lg text-sm hover:bg-slate-50"
          >
            Refresh
          </button>
        </div>
      </div>

      <div className="bg-white rounded-xl border border-slate-200 shadow-sm overflow-hidden">
        <table className="w-full text-sm min-w-[900px]">
          <thead className="bg-slate-50 border-b border-slate-200">
            <tr>
              {["Time", "Model", "Provider", "Tokens", "Cost", "Latency", "Status"].map((col) => (
                <th key={col} className="text-left px-4 py-3 font-medium text-slate-600 text-xs uppercase tracking-wide">
                  {col}
                </th>
              ))}
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {isLoading ? (
              <tr>
                <td colSpan={7} className="text-center py-8 text-slate-400 text-sm">
                  Loading…
                </td>
              </tr>
            ) : logList.length === 0 ? (
              <tr>
                <td colSpan={7} className="text-center py-8 text-slate-400 text-sm">
                  No logs yet.
                </td>
              </tr>
            ) : (
              logList.map((entry: LogEntry, i) => (
                <tr
                  key={entry.request_id ?? i}
                  className="hover:bg-slate-50 cursor-pointer"
                  onClick={() => setSelected(entry)}
                >
                  <td className="px-4 py-2.5 text-xs text-slate-400">
                    {new Date(entry.timestamp).toLocaleTimeString()}
                  </td>
                  <td className="px-4 py-2.5 font-mono text-xs max-w-[160px] truncate">
                    {entry.model}
                  </td>
                  <td className="px-4 py-2.5 text-slate-500">{entry.provider}</td>
                  <td className="px-4 py-2.5 text-slate-500">
                    {(entry.total_tokens ?? 0).toLocaleString()}
                  </td>
                  <td className="px-4 py-2.5 text-slate-500">${(entry.cost_usd ?? 0).toFixed(5)}</td>
                  <td className="px-4 py-2.5 text-slate-500">{entry.latency_ms}ms</td>
                  <td className="px-4 py-2.5">
                    <StatusBadge code={entry.status_code} />
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Log detail drawer */}
      {selected && (
        <div className="fixed inset-0 bg-black/30 flex justify-end z-50" onClick={() => setSelected(null)}>
          <div
            className="bg-white w-full max-w-lg h-full overflow-y-auto p-6 shadow-xl space-y-4"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="flex items-center justify-between">
              <h2 className="text-lg font-semibold">Log Detail</h2>
              <button onClick={() => setSelected(null)} className="text-slate-400 hover:text-slate-700">
                ✕
              </button>
            </div>
            <dl className="space-y-3 text-sm">
              {[
                ["Request ID", selected.request_id],
                ["Timestamp", new Date(selected.timestamp).toLocaleString()],
                ["Model", selected.model],
                ["Provider", selected.provider],
                ["Status", selected.status_code],
                ["Latency", `${selected.latency_ms}ms`],
                ["Prompt Tokens", selected.prompt_tokens ?? 0],
                ["Completion Tokens", selected.completion_tokens ?? 0],
                ["Total Tokens", selected.total_tokens ?? 0],
                ["Cost", `$${(selected.cost_usd ?? 0).toFixed(6)}`],
                ["Streaming", selected.is_streaming ? "Yes" : "No"],
                ["Finish Reason", selected.finish_reason || "—"],
                ["Error Code", selected.error_code || "—"],
                ["Error Message", selected.error_message || "—"],
              ].map(([k, v]) => (
                <div key={String(k)} className="flex gap-4">
                  <dt className="w-36 text-slate-500 shrink-0">{k}</dt>
                  <dd className="font-medium break-all">{String(v)}</dd>
                </div>
              ))}
            </dl>
          </div>
        </div>
      )}
    </div>
  );
}

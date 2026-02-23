"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { auditLogs, AuditEvent, AuditLogParams } from "@/lib/api";

const EVENT_TYPES = ["CREATE", "UPDATE", "DELETE", "LOGIN", "LOGOUT", "ACCESS_DENIED"];

function exportCSV(events: AuditEvent[]) {
  const headers: (keyof AuditEvent)[] = [
    "timestamp", "event_type", "action", "actor_email",
    "resource_type", "resource_name", "ip_address",
  ];
  const csv = [
    headers.join(","),
    ...events.map((r) => headers.map((h) => r[h] ?? "").join(",")),
  ].join("\n");
  const blob = new Blob([csv], { type: "text/csv" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `audit-logs-${new Date().toISOString().slice(0, 10)}.csv`;
  a.click();
  URL.revokeObjectURL(url);
}

function exportJSON(events: AuditEvent[]) {
  const blob = new Blob([JSON.stringify(events, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `audit-logs-${new Date().toISOString().slice(0, 10)}.json`;
  a.click();
  URL.revokeObjectURL(url);
}

type PendingFilters = {
  fromDate?: string;
  toDate?: string;
  event_type?: string;
  actor_id?: string;
};

export default function AuditPage() {
  const [filters, setFilters] = useState<AuditLogParams>({ limit: 50, page: 1 });
  const [pendingFilters, setPendingFilters] = useState<PendingFilters>({});

  const { data, isLoading, error } = useQuery({
    queryKey: ["audit-logs", filters],
    queryFn: () => auditLogs.list(filters),
    refetchInterval: 30_000,
  });

  const events = data?.events ?? [];
  const total = data?.total ?? 0;

  const dateInvalid =
    !!pendingFilters.fromDate &&
    !!pendingFilters.toDate &&
    pendingFilters.fromDate > pendingFilters.toDate;

  function handleApply() {
    if (dateInvalid) return;
    const next: AuditLogParams = { limit: filters.limit ?? 50, page: 1 };
    if (pendingFilters.fromDate) next.from = new Date(pendingFilters.fromDate).toISOString();
    if (pendingFilters.toDate) next.to = new Date(pendingFilters.toDate).toISOString();
    if (pendingFilters.event_type) next.event_type = pendingFilters.event_type;
    if (pendingFilters.actor_id) next.actor_id = pendingFilters.actor_id;
    setFilters(next);
  }

  function handleLimitChange(newLimit: number) {
    setFilters((prev) => ({ ...prev, limit: newLimit, page: 1 }));
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-slate-900">Audit Logs</h1>
        <div className="flex items-center gap-2">
          <button
            onClick={() => exportCSV(events)}
            disabled={events.length === 0}
            className="border border-slate-300 text-slate-700 px-3 py-1.5 rounded-lg text-sm hover:bg-slate-50 disabled:opacity-40 disabled:cursor-not-allowed"
          >
            Export CSV
          </button>
          <button
            onClick={() => exportJSON(events)}
            disabled={events.length === 0}
            className="border border-slate-300 text-slate-700 px-3 py-1.5 rounded-lg text-sm hover:bg-slate-50 disabled:opacity-40 disabled:cursor-not-allowed"
          >
            Export JSON
          </button>
        </div>
      </div>

      {/* Filter bar */}
      <div className="flex flex-wrap items-end gap-3 bg-white border border-slate-200 rounded-xl p-4">
        <div className="flex flex-col gap-1">
          <label htmlFor="from-date" className="text-xs text-slate-500">Date From</label>
          <input
            id="from-date"
            type="date"
            value={pendingFilters.fromDate ?? ""}
            onChange={(e) =>
              setPendingFilters((p) => ({ ...p, fromDate: e.target.value || undefined }))
            }
            className="border border-slate-300 rounded-lg px-3 py-1.5 text-sm"
          />
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor="to-date" className="text-xs text-slate-500">Date To</label>
          <input
            id="to-date"
            type="date"
            value={pendingFilters.toDate ?? ""}
            onChange={(e) =>
              setPendingFilters((p) => ({ ...p, toDate: e.target.value || undefined }))
            }
            className="border border-slate-300 rounded-lg px-3 py-1.5 text-sm"
          />
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor="event-type" className="text-xs text-slate-500">Event Type</label>
          <select
            id="event-type"
            value={pendingFilters.event_type ?? ""}
            onChange={(e) =>
              setPendingFilters((p) => ({ ...p, event_type: e.target.value || undefined }))
            }
            className="border border-slate-300 rounded-lg px-3 py-1.5 text-sm"
          >
            <option value="">All</option>
            {EVENT_TYPES.map((t) => (
              <option key={t} value={t}>{t}</option>
            ))}
          </select>
        </div>
        <div className="flex flex-col gap-1">
          <label htmlFor="actor-filter" className="text-xs text-slate-500">Actor</label>
          <input
            id="actor-filter"
            type="text"
            placeholder="Filter by actor email"
            value={pendingFilters.actor_id ?? ""}
            onChange={(e) =>
              setPendingFilters((p) => ({ ...p, actor_id: e.target.value || undefined }))
            }
            className="border border-slate-300 rounded-lg px-3 py-1.5 text-sm"
          />
        </div>
        <div className="flex flex-col justify-end gap-1">
          {dateInvalid && (
            <span className="text-xs text-red-500">&quot;From&quot; must be before &quot;To&quot;</span>
          )}
          <button
            onClick={handleApply}
            disabled={dateInvalid}
            className="bg-brand-600 text-white px-4 py-1.5 rounded-lg text-sm hover:bg-brand-700 disabled:opacity-40 disabled:cursor-not-allowed"
          >
            Apply
          </button>
        </div>
      </div>

      {/* Table */}
      <div className="bg-white rounded-xl border border-slate-200 shadow-sm overflow-hidden">
        <table className="w-full text-sm min-w-[900px]">
          <thead className="bg-slate-50 border-b border-slate-200">
            <tr>
              {["Time", "Event Type", "Action", "Actor", "Resource", "IP"].map((col) => (
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
            {isLoading ? (
              <tr>
                <td colSpan={6} className="text-center py-8 text-slate-400 text-sm">
                  Loading…
                </td>
              </tr>
            ) : error ? (
              <tr>
                <td colSpan={6} className="text-center py-8 text-red-500 text-sm">
                  Failed to load audit logs.
                </td>
              </tr>
            ) : events.length === 0 ? (
              <tr>
                <td colSpan={6} className="text-center py-8 text-slate-400 text-sm">
                  No audit logs found.
                </td>
              </tr>
            ) : (
              events.map((e, i) => (
                <tr key={e.request_id ?? i} className="hover:bg-slate-50">
                  <td className="px-4 py-2.5 text-xs text-slate-400">
                    {new Date(e.timestamp).toLocaleString()}
                  </td>
                  <td className="px-4 py-2.5">{e.event_type}</td>
                  <td className="px-4 py-2.5">{e.action}</td>
                  <td className="px-4 py-2.5">{e.actor_email || "—"}</td>
                  <td className="px-4 py-2.5 text-slate-500">
                    {e.resource_type} / {e.resource_name}
                  </td>
                  <td className="px-4 py-2.5 font-mono text-xs">{e.ip_address}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      {/* Footer */}
      <div className="flex items-center justify-between text-sm text-slate-500">
        <span>{total.toLocaleString()} total events</span>
        <div className="flex items-center gap-2">
          <span>Per page:</span>
          <select
            value={filters.limit ?? 50}
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
  );
}

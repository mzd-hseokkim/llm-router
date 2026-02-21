"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { usage, keys } from "@/lib/api";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
  PieChart,
  Pie,
  Cell,
  Legend,
} from "recharts";
import StatCard from "@/components/StatCard";

const COLORS = ["#0ea5e9", "#6366f1", "#f59e0b", "#10b981", "#ef4444", "#8b5cf6"];

export default function UsagePage() {
  const [period, setPeriod] = useState<"daily" | "weekly" | "monthly">("monthly");
  const [keyId, setKeyId] = useState<string>("");

  const { data: keyList = [] } = useQuery({
    queryKey: ["keys"],
    queryFn: () => keys.list(),
  });

  const { data: summary, isLoading: summaryLoading } = useQuery({
    queryKey: ["usage-summary", keyId, period],
    queryFn: () =>
      keyId
        ? usage.summary("key", keyId, period)
        : Promise.resolve(null),
    enabled: !!keyId,
  });

  const { data: spenders = [], isLoading: spendersLoading } = useQuery({
    queryKey: ["top-spenders", period],
    queryFn: () => usage.topSpenders(10, period),
    refetchInterval: 60_000,
  });

  const spenderChartData = spenders.map((s) => ({
    name: s.virtual_key_id.slice(0, 8) + "…",
    cost: parseFloat(s.cost_usd.toFixed(4)),
    requests: s.request_count,
  }));

  const modelData = summary?.by_model?.map((m) => ({
    name: m.model.split("/").pop() ?? m.model,
    value: parseFloat(m.cost_usd.toFixed(4)),
  })) ?? [];

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">Usage Analysis</h1>
        <p className="text-sm text-slate-500 mt-1">Select a period and optionally a virtual key</p>
      </div>

      {/* Filters */}
      <div className="flex flex-wrap gap-3">
        <select
          value={period}
          onChange={(e) => setPeriod(e.target.value as typeof period)}
          className="border border-slate-300 rounded-lg px-3 py-2 text-sm"
        >
          <option value="daily">Today</option>
          <option value="weekly">This week</option>
          <option value="monthly">This month</option>
        </select>

        <select
          value={keyId}
          onChange={(e) => setKeyId(e.target.value)}
          className="border border-slate-300 rounded-lg px-3 py-2 text-sm min-w-[200px]"
        >
          <option value="">— Select a key for details —</option>
          {keyList.map((k) => (
            <option key={k.id} value={k.id}>
              {k.name || k.key_prefix} ({k.key_prefix})
            </option>
          ))}
        </select>
      </div>

      {/* Key usage summary */}
      {keyId && (
        <div>
          {summaryLoading ? (
            <p className="text-sm text-slate-400">Loading…</p>
          ) : summary ? (
            <div className="space-y-6">
              <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard title="Requests" value={summary.total_requests.toLocaleString()} />
                <StatCard title="Total Tokens" value={summary.total_tokens.toLocaleString()} />
                <StatCard title="Cost" value={`$${summary.total_cost_usd.toFixed(4)}`} colorClass="text-brand-700" />
                <StatCard title="Errors" value={summary.error_count} colorClass={summary.error_count > 0 ? "text-red-600" : "text-slate-900"} />
              </div>

              {/* Model breakdown pie */}
              {modelData.length > 0 && (
                <div className="bg-white rounded-xl border border-slate-200 p-5 shadow-sm">
                  <h2 className="text-sm font-semibold text-slate-700 mb-4">Cost by Model</h2>
                  <ResponsiveContainer width="100%" height={260}>
                    <PieChart>
                      <Pie data={modelData} cx="50%" cy="50%" outerRadius={90} dataKey="value" label>
                        {modelData.map((_, i) => (
                          <Cell key={i} fill={COLORS[i % COLORS.length]} />
                        ))}
                      </Pie>
                      <Legend />
                      <Tooltip formatter={(v: number) => [`$${v}`, "Cost"]} />
                    </PieChart>
                  </ResponsiveContainer>
                </div>
              )}
            </div>
          ) : null}
        </div>
      )}

      {/* Top spenders */}
      <div className="bg-white rounded-xl border border-slate-200 p-5 shadow-sm">
        <h2 className="text-sm font-semibold text-slate-700 mb-4">Top Spenders ({period})</h2>
        {spendersLoading ? (
          <p className="text-sm text-slate-400">Loading…</p>
        ) : spenderChartData.length === 0 ? (
          <p className="text-sm text-slate-400 text-center py-6">No data for this period.</p>
        ) : (
          <ResponsiveContainer width="100%" height={280}>
            <BarChart data={spenderChartData} layout="vertical" margin={{ left: 60, right: 20 }}>
              <XAxis type="number" tick={{ fontSize: 11 }} tickFormatter={(v) => `$${v}`} />
              <YAxis type="category" dataKey="name" tick={{ fontSize: 11 }} width={80} />
              <Tooltip formatter={(v: number) => [`$${v}`, "Cost"]} />
              <Bar dataKey="cost" fill="#0ea5e9" radius={[0, 4, 4, 0]} />
            </BarChart>
          </ResponsiveContainer>
        )}
      </div>
    </div>
  );
}

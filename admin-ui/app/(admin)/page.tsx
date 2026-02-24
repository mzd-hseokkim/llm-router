"use client";

import { useQuery } from "@tanstack/react-query";
import { usage, keys, providerKeys, circuitBreakers, alerts, cache } from "@/lib/api";
import StatCard from "@/components/StatCard";
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

const COLORS = ["#0ea5e9", "#6366f1", "#f59e0b", "#10b981", "#ef4444"];

function fmt(n: number) {
  if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + "M";
  if (n >= 1_000) return (n / 1_000).toFixed(1) + "K";
  return n.toString();
}

export default function DashboardPage() {
  // We need a real entity_id for usage summary — for the overview we use a
  // "global" approach: fetch top spenders and aggregate.
  const { data: spenders = [] } = useQuery({
    queryKey: ["top-spenders"],
    queryFn: () => usage.topSpenders(5, "monthly"),
    refetchInterval: 30_000,
  });

  const { data: allKeys = [] } = useQuery({
    queryKey: ["keys"],
    queryFn: () => keys.list().then((r) => r.data ?? []),
    refetchInterval: 30_000,
  });

  const { data: pKeys = [] } = useQuery({
    queryKey: ["provider-keys"],
    queryFn: () => providerKeys.list(),
    refetchInterval: 30_000,
  });

  const { data: cbs = [] } = useQuery({
    queryKey: ["circuit-breakers"],
    queryFn: () => circuitBreakers.list(),
    refetchInterval: 30_000,
  });

  const { data: alertConfig } = useQuery({
    queryKey: ["alert-config"],
    queryFn: () => alerts.getConfig(),
    retry: false,
  });

  const { data: cacheStats } = useQuery({
    queryKey: ["cache-stats"],
    queryFn: () => cache.stats(),
    refetchInterval: 30_000,
  });

  const budgetThreshold = alertConfig?.conditions?.budget_threshold_pct ?? 80;
  const budgetWarnings = pKeys.filter(
    (p) =>
      p.monthly_budget_usd != null &&
      p.monthly_budget_usd > 0 &&
      (p.current_month_spend / p.monthly_budget_usd) * 100 >= budgetThreshold
  );

  const totalCost = spenders.reduce((s, sp) => s + sp.cost_usd, 0);
  const totalRequests = spenders.reduce((s, sp) => s + sp.request_count, 0);
  const activeKeys = allKeys.filter((k) => k.is_active).length;
  const activeProviders = pKeys.filter((p) => p.is_active).length;

  // Chart: top spenders bar
  const spenderChartData = spenders.map((s) => ({
    name: s.virtual_key_id.slice(0, 8) + "…",
    cost: parseFloat(s.cost_usd.toFixed(4)),
    requests: s.request_count,
  }));

  return (
    <div className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">Dashboard</h1>
        <p className="text-sm text-slate-500 mt-1">Monthly overview — auto-refreshes every 30s</p>
      </div>

      {/* Budget warning banner */}
      {budgetWarnings.length > 0 && (
        <div className="rounded-lg border border-amber-300 bg-amber-50 p-4">
          <p className="text-sm font-semibold text-amber-800 mb-2">
            ⚠️ Budget Alert — {budgetWarnings.length} provider key{budgetWarnings.length > 1 ? "s" : ""} near limit
          </p>
          <ul className="space-y-1">
            {budgetWarnings.map((p) => (
              <li key={p.id} className="text-sm text-amber-700">
                <span className="font-medium">{p.key_alias}</span>
                {" — "}
                {((p.current_month_spend / p.monthly_budget_usd!) * 100).toFixed(1)}% used
                {" "}(${p.current_month_spend.toFixed(2)} / ${p.monthly_budget_usd!.toFixed(2)})
              </li>
            ))}
          </ul>
        </div>
      )}

      {/* Stat cards */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        <StatCard
          title="Monthly Cost"
          value={`$${totalCost.toFixed(2)}`}
          colorClass="text-brand-700"
        />
        <StatCard title="Total Requests" value={fmt(totalRequests)} />
        <StatCard title="Active Keys" value={activeKeys} sub={`of ${allKeys.length} total`} />
        <StatCard title="Active Provider Keys" value={activeProviders} sub={`of ${pKeys.length} total`} />
        <StatCard
          title="Today's Cache Hit Rate"
          value={
            cacheStats == null
              ? "…"
              : cacheStats.hit_rate == null
              ? "N/A"
              : `${cacheStats.hit_rate.toFixed(1)}%`
          }
          sub={cacheStats != null ? `${cacheStats.hits} / ${cacheStats.total} requests` : undefined}
        />
      </div>

      {/* Charts */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Top spenders bar chart */}
        <div className="bg-white rounded-xl border border-slate-200 p-5 shadow-sm">
          <h2 className="text-sm font-semibold text-slate-700 mb-4">Top Spenders (this month)</h2>
          {spenderChartData.length === 0 ? (
            <p className="text-sm text-slate-400 py-8 text-center">No data</p>
          ) : (
            <ResponsiveContainer width="100%" height={220}>
              <BarChart data={spenderChartData} margin={{ left: -10 }}>
                <XAxis dataKey="name" tick={{ fontSize: 11 }} />
                <YAxis tick={{ fontSize: 11 }} />
                <Tooltip formatter={(v: number) => [`$${v}`, "Cost"]} />
                <Bar dataKey="cost" fill="#0ea5e9" radius={[4, 4, 0, 0]} />
              </BarChart>
            </ResponsiveContainer>
          )}
        </div>

        {/* Provider key status pie */}
        <div className="bg-white rounded-xl border border-slate-200 p-5 shadow-sm">
          <h2 className="text-sm font-semibold text-slate-700 mb-4">Provider Key Status</h2>
          {pKeys.length === 0 ? (
            <p className="text-sm text-slate-400 py-8 text-center">No provider keys</p>
          ) : (
            <ResponsiveContainer width="100%" height={220}>
              <PieChart>
                <Pie
                  data={[
                    { name: "Active", value: pKeys.filter((p) => p.is_active).length },
                    { name: "Inactive", value: pKeys.filter((p) => !p.is_active).length },
                  ]}
                  cx="50%"
                  cy="50%"
                  outerRadius={80}
                  dataKey="value"
                  label
                >
                  {[0, 1].map((i) => (
                    <Cell key={i} fill={COLORS[i]} />
                  ))}
                </Pie>
                <Legend />
                <Tooltip />
              </PieChart>
            </ResponsiveContainer>
          )}
        </div>
      </div>
      {/* Circuit Breaker status */}
      <div className="bg-white rounded-xl border border-slate-200 p-5 shadow-sm">
        <h2 className="text-sm font-semibold text-slate-700 mb-4">Circuit Breaker Status</h2>
        {!Array.isArray(cbs) || cbs.length === 0 ? (
          <p className="text-sm text-slate-400 text-center py-4">No circuit breakers tracked</p>
        ) : (
          <div className="flex flex-wrap gap-3">
            {cbs.map((cb) => {
              const stateStyles: Record<string, string> = {
                closed: "bg-emerald-100 text-emerald-800",
                open: "bg-red-100 text-red-800",
                half_open: "bg-amber-100 text-amber-800",
              };
              const style = stateStyles[cb.state] ?? "bg-slate-100 text-slate-700";
              return (
                <div key={cb.provider} className="flex items-center gap-2">
                  <span className="text-sm text-slate-600 font-medium capitalize">{cb.provider}</span>
                  <span className={`text-xs font-semibold px-2 py-0.5 rounded-full ${style}`}>
                    {cb.state.replace("_", " ").toUpperCase()}
                  </span>
                  {cb.failure_count > 0 && (
                    <span className="text-xs text-slate-400">{cb.failure_count} fail{cb.failure_count > 1 ? "s" : ""}</span>
                  )}
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

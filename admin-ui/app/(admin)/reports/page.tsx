"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { clsx } from "clsx";
import { reports, teams } from "@/lib/api";
import StatCard from "@/components/StatCard";

const CURRENT_PERIOD = new Date().toISOString().slice(0, 7); // "YYYY-MM"

type Tab = "chargeback" | "showback";

export default function ReportsPage() {
  const [period, setPeriod] = useState(CURRENT_PERIOD);
  const [tab, setTab] = useState<Tab>("chargeback");
  const [teamId, setTeamId] = useState("");
  const [exporting, setExporting] = useState(false);
  const [exportError, setExportError] = useState<string | null>(null);

  const { data: chargeback, isLoading: cbLoading } = useQuery({
    queryKey: ["chargeback", period],
    queryFn: () => reports.chargeback({ period }),
  });

  const { data: teamList = [], isError: teamsError } = useQuery({
    queryKey: ["teams"],
    queryFn: () => teams.list(),
  });

  const { data: showback, isLoading: sbLoading } = useQuery({
    queryKey: ["showback", teamId, period],
    queryFn: () => reports.showback(teamId, period),
    enabled: !!teamId,
  });

  async function handleCsvExport() {
    setExporting(true);
    setExportError(null);
    try {
      const res = await fetch(`/api/admin/reports/chargeback?period=${period}&format=csv`);
      if (!res.ok) throw new Error("내보내기에 실패했습니다.");
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `chargeback-${period}.csv`;
      a.click();
      URL.revokeObjectURL(url);
    } catch (err) {
      setExportError(err instanceof Error ? err.message : "내보내기에 실패했습니다.");
    } finally {
      setExporting(false);
    }
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Reports</h1>
          <p className="text-sm text-slate-500 mt-1">차지백 / 쇼백 비용 리포트</p>
        </div>
        <input
          type="month"
          value={period}
          onChange={(e) => setPeriod(e.target.value)}
          className="border border-slate-300 rounded-lg px-3 py-2 text-sm"
        />
      </div>

      {/* Tabs */}
      <div className="flex gap-1 border-b border-slate-200">
        {(["chargeback", "showback"] as Tab[]).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={clsx(
              "px-4 py-2 text-sm font-medium transition-colors",
              tab === t
                ? "border-b-2 border-brand-600 text-brand-700"
                : "text-slate-500 hover:text-slate-700"
            )}
          >
            {t === "chargeback" ? "Chargeback" : "Showback"}
          </button>
        ))}
      </div>

      {/* Chargeback Tab */}
      {tab === "chargeback" && (
        <div className="space-y-5">
          {cbLoading ? (
            <p className="text-sm text-slate-400">로딩 중…</p>
          ) : !chargeback ? (
            <p className="text-sm text-slate-400">데이터가 없습니다.</p>
          ) : (
            <>
              <div className="grid grid-cols-2 lg:grid-cols-3 gap-4">
                <StatCard
                  title="Total Cost"
                  value={`$${chargeback.summary.total_cost_usd.toFixed(4)}`}
                  colorClass="text-brand-700"
                />
                <StatCard
                  title="Total Tokens"
                  value={chargeback.summary.total_tokens.toLocaleString()}
                />
                <StatCard
                  title="Total Requests"
                  value={chargeback.summary.total_requests.toLocaleString()}
                />
              </div>

              <div className="bg-white rounded-xl border border-slate-200 shadow-sm">
                <div className="flex items-center justify-between px-5 py-4 border-b border-slate-100">
                  <h2 className="text-sm font-semibold text-slate-700">
                    팀별 차지백 ({chargeback.period})
                  </h2>
                  <div className="flex items-center gap-2">
                    {exportError && (
                      <p className="text-xs text-red-500">{exportError}</p>
                    )}
                    <button
                      onClick={handleCsvExport}
                      disabled={exporting}
                      className="text-xs px-3 py-1.5 bg-slate-100 hover:bg-slate-200 rounded-md text-slate-700 transition-colors disabled:opacity-50"
                    >
                      {exporting ? "내보내는 중…" : "CSV 내보내기"}
                    </button>
                  </div>
                </div>
                {(!chargeback.by_team || chargeback.by_team.length === 0) ? (
                  <p className="text-sm text-slate-400 text-center py-8">
                    해당 기간에 데이터가 없습니다.
                  </p>
                ) : (
                  <div className="overflow-x-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="text-xs text-slate-500 uppercase tracking-wide bg-slate-50">
                          <th className="px-5 py-3 text-left">Team</th>
                          <th className="px-5 py-3 text-right">Cost ($)</th>
                          <th className="px-5 py-3 text-right">Markup ($)</th>
                          <th className="px-5 py-3 text-right">Total Charged ($)</th>
                          <th className="px-5 py-3 text-right">Tokens</th>
                          <th className="px-5 py-3 text-right">Requests</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-slate-100">
                        {chargeback.by_team.map((entry) => (
                          <tr key={entry.team_id} className="hover:bg-slate-50 transition-colors">
                            <td className="px-5 py-3 font-medium text-slate-800">
                              {entry.team_name || entry.team_id}
                            </td>
                            <td className="px-5 py-3 text-right text-slate-600">
                              {entry.cost_usd.toFixed(4)}
                            </td>
                            <td className="px-5 py-3 text-right text-slate-600">
                              {entry.markup_usd.toFixed(4)}
                            </td>
                            <td className="px-5 py-3 text-right font-semibold text-slate-800">
                              {entry.total_charged_usd.toFixed(4)}
                            </td>
                            <td className="px-5 py-3 text-right text-slate-600">
                              {entry.tokens.toLocaleString()}
                            </td>
                            <td className="px-5 py-3 text-right text-slate-600">
                              {entry.requests.toLocaleString()}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            </>
          )}
        </div>
      )}

      {/* Showback Tab */}
      {tab === "showback" && (
        <div className="space-y-5">
          {teamsError ? (
            <p className="text-sm text-red-500">팀 목록을 불러오지 못했습니다.</p>
          ) : (
            <select
              value={teamId}
              onChange={(e) => setTeamId(e.target.value)}
              className="border border-slate-300 rounded-lg px-3 py-2 text-sm min-w-[220px]"
            >
              <option value="">— 팀을 선택하세요 —</option>
              {teamList.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.name}
                </option>
              ))}
            </select>
          )}

          {!teamId ? (
            <p className="text-sm text-slate-400">팀을 선택하면 쇼백 데이터가 표시됩니다.</p>
          ) : sbLoading ? (
            <p className="text-sm text-slate-400">로딩 중…</p>
          ) : !showback ? (
            <p className="text-sm text-slate-400">데이터가 없습니다.</p>
          ) : (
            <>
              <div className="grid grid-cols-2 lg:grid-cols-3 gap-4">
                <StatCard
                  title="Total Cost"
                  value={`$${showback.cost_usd.toFixed(4)}`}
                  colorClass="text-brand-700"
                />
                <StatCard title="Total Tokens" value={showback.tokens.toLocaleString()} />
                <StatCard title="Total Requests" value={showback.requests.toLocaleString()} />
              </div>

              <div className="bg-white rounded-xl border border-slate-200 shadow-sm">
                <div className="px-5 py-4 border-b border-slate-100">
                  <h2 className="text-sm font-semibold text-slate-700">
                    모델별 비용 ({showback.team_name})
                  </h2>
                </div>
                {showback.by_model && showback.by_model.length > 0 ? (
                  <div className="overflow-x-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="text-xs text-slate-500 uppercase tracking-wide bg-slate-50">
                          <th className="px-5 py-3 text-left">Model</th>
                          <th className="px-5 py-3 text-right">Cost ($)</th>
                          <th className="px-5 py-3 text-right">Tokens</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-slate-100">
                        {showback.by_model.map((m) => (
                          <tr key={m.Model} className="hover:bg-slate-50 transition-colors">
                            <td className="px-5 py-3 font-medium text-slate-800">{m.Model}</td>
                            <td className="px-5 py-3 text-right text-slate-600">
                              {m.CostUSD.toFixed(4)}
                            </td>
                            <td className="px-5 py-3 text-right text-slate-600">
                              {m.Tokens.toLocaleString()}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <p className="text-sm text-slate-400 text-center py-8">
                    모델별 데이터가 없습니다.
                  </p>
                )}
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}

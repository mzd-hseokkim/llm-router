"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { budgets, Budget, CreateBudgetPayload } from "@/lib/api";

const ENTITY_TYPES = ["key", "user", "team", "org"] as const;
const PERIODS = ["hourly", "daily", "weekly", "monthly", "lifetime"] as const;

function getBarColor(pct: number): string {
  if (pct >= 100) return "bg-red-500";
  if (pct >= 80) return "bg-orange-500";
  if (pct >= 60) return "bg-yellow-400";
  return "bg-green-500";
}

function SpendBar({ budget }: { budget: Budget }) {
  const limit = budget.hard_limit_usd ?? budget.soft_limit_usd;
  if (limit == null || limit === 0) {
    return <span className="text-xs text-slate-400">한도 없음</span>;
  }
  const pct = Math.min((budget.current_spend / limit) * 100, 110);
  const displayPct = Math.round((budget.current_spend / limit) * 100);
  return (
    <div className="space-y-1">
      <div className="w-full bg-slate-100 rounded-full h-2">
        <div
          className={`h-2 rounded-full ${getBarColor(displayPct)}`}
          style={{ width: `${Math.min(pct, 100)}%` }}
        />
      </div>
      <p className="text-xs text-slate-500">
        ${budget.current_spend.toFixed(2)} / ${limit.toFixed(2)} ({displayPct}%)
      </p>
    </div>
  );
}

function BudgetModal({
  initial,
  onClose,
}: {
  initial?: Budget;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [form, setForm] = useState<CreateBudgetPayload>({
    entity_type: initial?.entity_type ?? "key",
    entity_id: initial?.entity_id ?? "",
    period: initial?.period ?? "monthly",
    soft_limit_usd: initial?.soft_limit_usd ?? undefined,
    hard_limit_usd: initial?.hard_limit_usd ?? undefined,
  });
  const [error, setError] = useState("");

  const mutation = useMutation({
    mutationFn: () => budgets.create(form),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["budgets"] });
      onClose();
    },
    onError: (e: Error) => setError(e.message),
  });

  const isValidUUID =
    form.entity_id === "" ||
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(form.entity_id);

  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-md p-6 space-y-4">
        <h2 className="text-lg font-semibold">예산 설정</h2>

        {error && <p className="text-sm text-red-600">{error}</p>}

        <div className="space-y-3">
          <label className="block">
            <span className="text-sm font-medium text-slate-700">Entity Type</span>
            <select
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              value={form.entity_type}
              onChange={(e) => setForm((f) => ({ ...f, entity_type: e.target.value }))}
            >
              {ENTITY_TYPES.map((t) => (
                <option key={t} value={t}>{t}</option>
              ))}
            </select>
          </label>

          <label className="block">
            <span className="text-sm font-medium text-slate-700">Entity ID (UUID)</span>
            <input
              className={`mt-1 block w-full border rounded-lg px-3 py-2 text-sm ${
                !isValidUUID ? "border-red-400" : "border-slate-300"
              }`}
              placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              value={form.entity_id}
              onChange={(e) => setForm((f) => ({ ...f, entity_id: e.target.value }))}
            />
            {!isValidUUID && (
              <p className="text-xs text-red-500 mt-1">유효한 UUID 형식이 아닙니다</p>
            )}
          </label>

          <label className="block">
            <span className="text-sm font-medium text-slate-700">기간</span>
            <select
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              value={form.period}
              onChange={(e) => setForm((f) => ({ ...f, period: e.target.value }))}
            >
              {PERIODS.map((p) => (
                <option key={p} value={p}>{p}</option>
              ))}
            </select>
          </label>

          <label className="block">
            <span className="text-sm font-medium text-slate-700">Soft 한도 ($)</span>
            <input
              type="number"
              step="0.01"
              min="0"
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              placeholder="미설정"
              value={form.soft_limit_usd ?? ""}
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  soft_limit_usd: e.target.value ? Number(e.target.value) : undefined,
                }))
              }
            />
          </label>

          <label className="block">
            <span className="text-sm font-medium text-slate-700">Hard 한도 ($)</span>
            <input
              type="number"
              step="0.01"
              min="0"
              className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
              placeholder="미설정"
              value={form.hard_limit_usd ?? ""}
              onChange={(e) =>
                setForm((f) => ({
                  ...f,
                  hard_limit_usd: e.target.value ? Number(e.target.value) : undefined,
                }))
              }
            />
          </label>
        </div>

        <div className="flex gap-3 pt-2">
          <button
            onClick={() => mutation.mutate()}
            disabled={!form.entity_id || !isValidUUID || mutation.isPending}
            className="flex-1 bg-brand-600 text-white py-2 rounded-lg text-sm font-medium disabled:opacity-50"
          >
            {mutation.isPending ? "저장 중…" : "저장"}
          </button>
          <button
            onClick={onClose}
            className="flex-1 border border-slate-300 text-slate-700 py-2 rounded-lg text-sm font-medium"
          >
            취소
          </button>
        </div>
      </div>
    </div>
  );
}

export default function BudgetsPage() {
  const qc = useQueryClient();
  const [entityType, setEntityType] = useState<string>("key");
  const [entityId, setEntityId] = useState("");
  const [searchKey, setSearchKey] = useState<{ type: string; id: string } | null>(null);
  const [showModal, setShowModal] = useState(false);
  const [editTarget, setEditTarget] = useState<Budget | undefined>(undefined);

  const isValidUUID =
    entityId === "" ||
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(entityId);

  const { data: budgetList = [], isLoading, error } = useQuery({
    queryKey: ["budgets", searchKey?.type, searchKey?.id],
    queryFn: () =>
      searchKey ? budgets.list(searchKey.type, searchKey.id) : Promise.resolve([]),
    enabled: searchKey !== null,
  });

  function handleSearch() {
    if (!entityId || !isValidUUID) return;
    setSearchKey({ type: entityType, id: entityId });
  }

  function openCreate() {
    setEditTarget(undefined);
    setShowModal(true);
  }

  function openEdit(b: Budget) {
    setEditTarget(b);
    setShowModal(true);
  }

  return (
    <div className="space-y-6">
      {/* 헤더 */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">예산 관리</h1>
          <p className="text-sm text-slate-500 mt-1">팀·키·사용자별 예산 한도 설정</p>
        </div>
        <button
          onClick={openCreate}
          className="bg-brand-600 text-white px-4 py-2 rounded-lg text-sm font-medium hover:bg-brand-700"
        >
          + 새 예산
        </button>
      </div>

      {/* 검색 폼 */}
      <div className="bg-white rounded-xl border border-slate-200 shadow-sm p-4">
        <div className="flex items-end gap-3">
          <div className="flex-none">
            <label className="block text-xs font-medium text-slate-600 mb-1">Entity Type</label>
            <select
              className="border border-slate-300 rounded-lg px-3 py-2 text-sm"
              value={entityType}
              onChange={(e) => setEntityType(e.target.value)}
            >
              {ENTITY_TYPES.map((t) => (
                <option key={t} value={t}>{t}</option>
              ))}
            </select>
          </div>
          <div className="flex-1">
            <label className="block text-xs font-medium text-slate-600 mb-1">Entity ID (UUID)</label>
            <input
              className={`w-full border rounded-lg px-3 py-2 text-sm ${
                !isValidUUID ? "border-red-400" : "border-slate-300"
              }`}
              placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
              value={entityId}
              onChange={(e) => setEntityId(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && handleSearch()}
            />
            {!isValidUUID && (
              <p className="text-xs text-red-500 mt-1">유효한 UUID 형식이 아닙니다</p>
            )}
          </div>
          <button
            onClick={handleSearch}
            disabled={!entityId || !isValidUUID}
            className="px-4 py-2 bg-slate-800 text-white rounded-lg text-sm font-medium hover:bg-slate-700 disabled:opacity-50"
          >
            조회
          </button>
        </div>
      </div>

      {/* 목록 테이블 */}
      {searchKey && (
        <div className="bg-white rounded-xl border border-slate-200 shadow-sm overflow-hidden">
          {isLoading ? (
            <p className="text-sm text-slate-400 py-8 text-center">Loading…</p>
          ) : error ? (
            <p className="text-sm text-red-500 py-8 text-center">
              오류가 발생했습니다: {(error as Error).message}
            </p>
          ) : (
            <table className="w-full text-sm">
              <thead className="bg-slate-50 border-b border-slate-200">
                <tr>
                  {["기간", "Soft 한도", "Hard 한도", "사용량", "기간 종료", "리셋"].map((col) => (
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
                {budgetList.map((b: Budget) => (
                  <tr key={b.id} className="hover:bg-slate-50">
                    <td className="px-4 py-3">
                      <button
                        onClick={() => openEdit(b)}
                        className="font-medium text-brand-600 hover:underline"
                      >
                        {b.period}
                      </button>
                    </td>
                    <td className="px-4 py-3 text-slate-500">
                      {b.soft_limit_usd != null ? `$${b.soft_limit_usd.toFixed(2)}` : "미설정"}
                    </td>
                    <td className="px-4 py-3 text-slate-500">
                      {b.hard_limit_usd != null ? `$${b.hard_limit_usd.toFixed(2)}` : "미설정"}
                    </td>
                    <td className="px-4 py-3 min-w-[200px]">
                      <SpendBar budget={b} />
                    </td>
                    <td className="px-4 py-3 text-slate-400 text-xs">
                      {new Date(b.period_end).toLocaleDateString("ko-KR")}
                    </td>
                    <td className="px-4 py-3">
                      <div className="relative group inline-block">
                        <button
                          disabled
                          className="text-xs text-slate-400 cursor-not-allowed"
                          aria-label="기간 종료 시 자동으로 리셋됩니다"
                        >
                          🔒 리셋
                        </button>
                        <div className="absolute bottom-full left-1/2 -translate-x-1/2 mb-1 px-2 py-1 bg-slate-800 text-white text-xs rounded whitespace-nowrap opacity-0 group-hover:opacity-100 pointer-events-none transition-opacity">
                          기간 종료 시 자동으로 리셋됩니다
                        </div>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}

          {!isLoading && !error && budgetList.length === 0 && (
            <p className="text-center text-slate-400 py-8 text-sm">
              해당 엔티티의 예산이 없습니다.
            </p>
          )}
        </div>
      )}

      {showModal && (
        <BudgetModal
          initial={editTarget}
          onClose={() => {
            setShowModal(false);
            setEditTarget(undefined);
          }}
        />
      )}
    </div>
  );
}

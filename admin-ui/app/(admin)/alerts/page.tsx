"use client";

import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  alerts,
  AlertConfig,
  AlertChannelConfig,
  AlertConditions,
} from "@/lib/api";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function Toggle({
  checked,
  onChange,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
}) {
  return (
    <button
      type="button"
      onClick={() => onChange(!checked)}
      className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${
        checked ? "bg-brand-600" : "bg-slate-300"
      }`}
    >
      <span
        className={`inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform ${
          checked ? "translate-x-6" : "translate-x-1"
        }`}
      />
    </button>
  );
}

function SectionCard({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border border-slate-200 bg-white shadow-sm">
      <div className="border-b border-slate-100 px-6 py-4">
        <h2 className="text-base font-semibold text-slate-900">{title}</h2>
        {description && (
          <p className="mt-0.5 text-sm text-slate-500">{description}</p>
        )}
      </div>
      <div className="px-6 py-5">{children}</div>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Channel row with test button
// ---------------------------------------------------------------------------

function ChannelTestResult({
  result,
}: {
  result: { ok: boolean; message: string } | null;
}) {
  if (!result) return null;
  return (
    <span
      className={`text-xs ml-2 ${result.ok ? "text-green-600" : "text-red-500"}`}
    >
      {result.ok ? "✓" : "✗"} {result.message}
    </span>
  );
}

// ---------------------------------------------------------------------------
// Channels Section
// ---------------------------------------------------------------------------

function ChannelsCard({
  channels,
  onChange,
  onTest,
  testPending,
}: {
  channels: AlertChannelConfig;
  onChange: (c: AlertChannelConfig) => void;
  onTest: (channel: string) => void;
  testPending: string | null;
}) {
  const [testResults, setTestResults] = useState<
    Record<string, { ok: boolean; message: string } | null>
  >({});

  function handleTest(channel: string) {
    onTest(channel);
  }

  const slack = channels.slack ?? { webhook_url: "", enabled: false };
  const email = channels.email ?? { addresses: [], enabled: false };
  const webhook = channels.webhook ?? { url: "", enabled: false };

  return (
    <SectionCard
      title="채널 설정"
      description="알림을 수신할 채널을 설정합니다."
    >
      <div className="space-y-6">
        {/* Slack */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-slate-700">Slack</span>
            <Toggle
              checked={slack.enabled}
              onChange={(v) =>
                onChange({ ...channels, slack: { ...slack, enabled: v } })
              }
            />
          </div>
          {slack.enabled && (
            <div className="flex items-center gap-2">
              <input
                type="url"
                value={slack.webhook_url}
                onChange={(e) =>
                  onChange({
                    ...channels,
                    slack: { ...slack, webhook_url: e.target.value },
                  })
                }
                placeholder="https://hooks.slack.com/services/..."
                className="flex-1 rounded border border-slate-300 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
              />
              <button
                onClick={() => handleTest("slack")}
                disabled={testPending === "slack" || !slack.webhook_url}
                className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50 disabled:opacity-50"
              >
                {testPending === "slack" ? "발송 중…" : "테스트"}
              </button>
            </div>
          )}
        </div>

        {/* Email */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-slate-700">Email</span>
            <Toggle
              checked={email.enabled}
              onChange={(v) =>
                onChange({ ...channels, email: { ...email, enabled: v } })
              }
            />
          </div>
          {email.enabled && (
            <div className="space-y-1">
              <textarea
                value={email.addresses.join("\n")}
                onChange={(e) =>
                  onChange({
                    ...channels,
                    email: {
                      ...email,
                      addresses: e.target.value
                        .split("\n")
                        .map((a) => a.trim())
                        .filter(Boolean),
                    },
                  })
                }
                rows={3}
                placeholder="admin@example.com&#10;ops@example.com"
                className="w-full rounded border border-slate-300 px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
              />
              <div className="flex items-center gap-2">
                <p className="text-xs text-slate-400 flex-1">
                  수신 주소 (한 줄에 하나씩)
                </p>
                <button
                  onClick={() => handleTest("email")}
                  disabled={
                    testPending === "email" || email.addresses.length === 0
                  }
                  className="rounded border border-slate-300 px-3 py-1 text-sm hover:bg-slate-50 disabled:opacity-50"
                >
                  {testPending === "email" ? "발송 중…" : "테스트"}
                </button>
              </div>
            </div>
          )}
        </div>

        {/* Webhook */}
        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium text-slate-700">Webhook</span>
            <Toggle
              checked={webhook.enabled}
              onChange={(v) =>
                onChange({ ...channels, webhook: { ...webhook, enabled: v } })
              }
            />
          </div>
          {webhook.enabled && (
            <div className="flex items-center gap-2">
              <input
                type="url"
                value={webhook.url}
                onChange={(e) =>
                  onChange({
                    ...channels,
                    webhook: { ...webhook, url: e.target.value },
                  })
                }
                placeholder="https://your-endpoint.example.com/webhook"
                className="flex-1 rounded border border-slate-300 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
              />
              <button
                onClick={() => handleTest("webhook")}
                disabled={testPending === "webhook" || !webhook.url}
                className="rounded border border-slate-300 px-3 py-1.5 text-sm hover:bg-slate-50 disabled:opacity-50"
              >
                {testPending === "webhook" ? "발송 중…" : "테스트"}
              </button>
            </div>
          )}
        </div>
      </div>
    </SectionCard>
  );
}

// ---------------------------------------------------------------------------
// Conditions Section
// ---------------------------------------------------------------------------

function ConditionsCard({
  conditions,
  onChange,
}: {
  conditions: AlertConditions;
  onChange: (c: AlertConditions) => void;
}) {
  return (
    <SectionCard
      title="알림 조건"
      description="각 임계값을 초과할 때 알림을 발송합니다."
    >
      <div className="space-y-4">
        <div className="grid grid-cols-3 gap-4">
          <div>
            <label className="block text-sm font-medium text-slate-700 mb-1">
              예산 임박 알림 (%)
            </label>
            <input
              type="number"
              min={0}
              max={100}
              value={conditions.budget_threshold_pct ?? ""}
              onChange={(e) =>
                onChange({
                  ...conditions,
                  budget_threshold_pct: e.target.value
                    ? Number(e.target.value)
                    : undefined,
                })
              }
              placeholder="80"
              className="w-full rounded border border-slate-300 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-slate-700 mb-1">
              에러율 임계값 (%)
            </label>
            <input
              type="number"
              min={0}
              max={100}
              value={conditions.error_rate_threshold ?? ""}
              onChange={(e) =>
                onChange({
                  ...conditions,
                  error_rate_threshold: e.target.value
                    ? Number(e.target.value)
                    : undefined,
                })
              }
              placeholder="5"
              className="w-full rounded border border-slate-300 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-slate-700 mb-1">
              응답시간 임계값 (ms)
            </label>
            <input
              type="number"
              min={0}
              value={conditions.latency_threshold_ms ?? ""}
              onChange={(e) =>
                onChange({
                  ...conditions,
                  latency_threshold_ms: e.target.value
                    ? Number(e.target.value)
                    : undefined,
                })
              }
              placeholder="2000"
              className="w-full rounded border border-slate-300 px-3 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-brand-500"
            />
          </div>
        </div>
      </div>
    </SectionCard>
  );
}

// ---------------------------------------------------------------------------
// History Section
// ---------------------------------------------------------------------------

function HistoryCard() {
  const { data: history = [], isLoading } = useQuery({
    queryKey: ["alerts-history"],
    queryFn: () => alerts.history(50),
    refetchInterval: 30_000,
  });

  return (
    <SectionCard title="최근 발송 히스토리">
      {isLoading ? (
        <p className="text-sm text-slate-400">로딩 중…</p>
      ) : history.length === 0 ? (
        <p className="text-sm text-slate-400">발송 기록이 없습니다.</p>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-100 text-left text-xs font-medium text-slate-500 uppercase tracking-wide">
                <th className="pb-2 pr-4">발송 시각</th>
                <th className="pb-2 pr-4">이벤트 타입</th>
                <th className="pb-2 pr-4">채널</th>
                <th className="pb-2 pr-4">상태</th>
                <th className="pb-2">오류</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-50">
              {history.map((entry) => (
                <tr key={entry.id} className="hover:bg-slate-50">
                  <td className="py-2 pr-4 text-slate-500 whitespace-nowrap">
                    {new Date(entry.sent_at).toLocaleString("ko-KR")}
                  </td>
                  <td className="py-2 pr-4 font-mono text-xs text-slate-700">
                    {entry.event_type}
                  </td>
                  <td className="py-2 pr-4 text-slate-700">{entry.channel}</td>
                  <td className="py-2 pr-4">
                    <span
                      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${
                        entry.status === "sent"
                          ? "bg-green-100 text-green-700"
                          : "bg-red-100 text-red-700"
                      }`}
                    >
                      {entry.status}
                    </span>
                  </td>
                  <td className="py-2 text-xs text-slate-400">
                    {entry.error ?? "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </SectionCard>
  );
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export default function AlertsPage() {
  const qc = useQueryClient();

  const { data: config, isLoading, error } = useQuery({
    queryKey: ["alerts-config"],
    queryFn: alerts.getConfig,
  });

  const [channels, setChannels] = useState<AlertChannelConfig>({});
  const [conditions, setConditions] = useState<AlertConditions>({});
  const [enabled, setEnabled] = useState(true);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [saveSuccess, setSaveSuccess] = useState(false);
  const [testPending, setTestPending] = useState<string | null>(null);
  const [testResults, setTestResults] = useState<
    Record<string, { ok: boolean; message: string }>
  >({});

  useEffect(() => {
    if (config) {
      setChannels(config.channels ?? {});
      setConditions(config.conditions ?? {});
      setEnabled(config.enabled ?? true);
    }
  }, [config]);

  const saveMutation = useMutation({
    mutationFn: (payload: Partial<AlertConfig>) => alerts.updateConfig(payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["alerts-config"] });
      setSaveError(null);
      setSaveSuccess(true);
      setTimeout(() => setSaveSuccess(false), 3000);
    },
    onError: (e: Error) => setSaveError(e.message),
  });

  const testMutation = useMutation({
    mutationFn: (channel: string) => alerts.test(channel),
    onSuccess: (data, channel) => {
      setTestResults((prev) => ({
        ...prev,
        [channel]: { ok: true, message: data.status },
      }));
      setTestPending(null);
    },
    onError: (e: Error, channel) => {
      setTestResults((prev) => ({
        ...prev,
        [channel]: { ok: false, message: e.message },
      }));
      setTestPending(null);
    },
  });

  function handleTest(channel: string) {
    setTestPending(channel);
    testMutation.mutate(channel);
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-64 text-slate-400 text-sm">
        알림 설정 로딩 중…
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-64 text-red-500 text-sm">
        알림 설정을 불러오지 못했습니다: {(error as Error).message}
      </div>
    );
  }

  return (
    <div className="max-w-3xl mx-auto space-y-6 py-8 px-4">
      <div className="flex items-start justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">알림 설정</h1>
          <p className="mt-1 text-sm text-slate-500">
            Slack, Email, Webhook 채널과 알림 조건을 설정합니다.
          </p>
        </div>
        <div className="flex items-center gap-2 mt-1">
          <span className="text-sm text-slate-600">알림 활성화</span>
          <Toggle checked={enabled} onChange={setEnabled} />
        </div>
      </div>

      <ChannelsCard
        channels={channels}
        onChange={setChannels}
        onTest={handleTest}
        testPending={testPending}
      />

      {/* Test results */}
      {Object.entries(testResults).length > 0 && (
        <div className="rounded-md border border-slate-200 bg-slate-50 px-4 py-3 text-sm space-y-1">
          {Object.entries(testResults).map(([channel, result]) => (
            <div key={channel}>
              <span className="font-medium capitalize">{channel}:</span>{" "}
              <span className={result.ok ? "text-green-600" : "text-red-500"}>
                {result.ok ? "✓" : "✗"} {result.message}
              </span>
            </div>
          ))}
        </div>
      )}

      <ConditionsCard conditions={conditions} onChange={setConditions} />

      <div className="flex items-center justify-between">
        <div>
          {saveError && (
            <p className="text-sm text-red-500">저장 실패: {saveError}</p>
          )}
          {saveSuccess && (
            <p className="text-sm text-green-600">설정이 저장되었습니다.</p>
          )}
        </div>
        <button
          onClick={() =>
            saveMutation.mutate({ channels, conditions, enabled })
          }
          disabled={saveMutation.isPending}
          className="rounded bg-brand-600 px-5 py-2 text-sm font-medium text-white hover:bg-brand-700 disabled:opacity-50"
        >
          {saveMutation.isPending ? "저장 중…" : "저장"}
        </button>
      </div>

      <HistoryCard />
    </div>
  );
}

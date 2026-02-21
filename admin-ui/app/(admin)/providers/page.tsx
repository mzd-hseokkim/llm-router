"use client";

import { useQuery } from "@tanstack/react-query";
import { providerKeys, ProviderKey } from "@/lib/api";

const PROVIDER_COLORS: Record<string, string> = {
  openai: "bg-green-100 text-green-800",
  anthropic: "bg-orange-100 text-orange-800",
  gemini: "bg-blue-100 text-blue-800",
  azure: "bg-sky-100 text-sky-800",
  mistral: "bg-purple-100 text-purple-800",
  cohere: "bg-pink-100 text-pink-800",
  bedrock: "bg-yellow-100 text-yellow-800",
};

function ProviderBadge({ name }: { name: string }) {
  const cls = PROVIDER_COLORS[name.toLowerCase()] ?? "bg-slate-100 text-slate-700";
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${cls}`}>
      {name}
    </span>
  );
}

export default function ProvidersPage() {
  const { data: pKeys = [], isLoading } = useQuery({
    queryKey: ["provider-keys"],
    queryFn: () => providerKeys.list(),
    refetchInterval: 30_000,
  });

  // Group by provider
  const byProvider = pKeys.reduce<Record<string, ProviderKey[]>>((acc, pk) => {
    (acc[pk.provider] ??= []).push(pk);
    return acc;
  }, {});

  const providers = Object.keys(byProvider).sort();

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-slate-900">Providers</h1>
        <p className="text-sm text-slate-500 mt-1">Provider API key inventory</p>
      </div>

      {isLoading ? (
        <p className="text-sm text-slate-400">Loading…</p>
      ) : providers.length === 0 ? (
        <div className="bg-white rounded-xl border border-slate-200 p-8 text-center text-sm text-slate-400">
          No provider keys configured. Add provider keys via the Admin API.
        </div>
      ) : (
        <div className="space-y-4">
          {providers.map((provider) => (
            <div key={provider} className="bg-white rounded-xl border border-slate-200 shadow-sm overflow-hidden">
              <div className="px-5 py-3 bg-slate-50 border-b border-slate-200 flex items-center gap-3">
                <ProviderBadge name={provider} />
                <span className="text-sm text-slate-500">
                  {byProvider[provider].length} key(s)
                </span>
              </div>
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-slate-100">
                    {["Alias", "Preview", "Status", "Weight", "Use Count", "Monthly Spend", "Monthly Budget"].map(
                      (col) => (
                        <th key={col} className="text-left px-4 py-2 font-medium text-slate-500 text-xs">
                          {col}
                        </th>
                      )
                    )}
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-50">
                  {byProvider[provider].map((pk: ProviderKey) => (
                    <tr key={pk.id} className="hover:bg-slate-50">
                      <td className="px-4 py-2.5 font-medium">{pk.key_alias}</td>
                      <td className="px-4 py-2.5 font-mono text-xs text-slate-400">{pk.key_preview}</td>
                      <td className="px-4 py-2.5">
                        <span
                          className={`inline-flex items-center gap-1 text-xs font-medium ${
                            pk.is_active ? "text-green-700" : "text-slate-400"
                          }`}
                        >
                          <span
                            className={`w-1.5 h-1.5 rounded-full ${
                              pk.is_active ? "bg-green-500" : "bg-slate-300"
                            }`}
                          />
                          {pk.is_active ? "Active" : "Inactive"}
                        </span>
                      </td>
                      <td className="px-4 py-2.5 text-slate-500">{pk.weight}</td>
                      <td className="px-4 py-2.5 text-slate-500">{pk.use_count.toLocaleString()}</td>
                      <td className="px-4 py-2.5 text-slate-500">
                        ${pk.current_month_spend.toFixed(4)}
                      </td>
                      <td className="px-4 py-2.5 text-slate-500">
                        {pk.monthly_budget_usd != null ? `$${pk.monthly_budget_usd}` : "—"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

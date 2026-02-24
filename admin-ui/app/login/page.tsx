"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";

type OAuthProvider = { name: string; label: string; icon: string };

const PROVIDER_LABELS: Record<string, { label: string; icon: string }> = {
  google: { label: "Google", icon: "G" },
  github: { label: "GitHub", icon: "🐙" },
  okta:   { label: "Okta",   icon: "O" },
  azure:  { label: "Microsoft", icon: "M" },
};

export default function LoginPage() {
  const router = useRouter();
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [oauthProviders, setOauthProviders] = useState<OAuthProvider[]>([]);

  // Load available OAuth providers from the gateway
  useEffect(() => {
    fetch("/api/auth/providers")
      .then((r) => r.json())
      .then((data: { providers?: string[] }) => {
        const providers = (data.providers ?? []).map((name) => ({
          name,
          label: PROVIDER_LABELS[name]?.label ?? name,
          icon:  PROVIDER_LABELS[name]?.icon  ?? name[0].toUpperCase(),
        }));
        setOauthProviders(providers);
      })
      .catch(() => { /* providers endpoint not available — fall back to password-only UI */ });
  }, []);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const res = await fetch("/api/auth/login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        setError(data.error ?? "Login failed");
        return;
      }
      const data = await res.json().catch(() => ({}));
      if (data.password_changed === false) {
        router.push("/change-password");
      } else {
        router.push("/");
      }
    } catch {
      setError("Network error — please try again");
    } finally {
      setLoading(false);
    }
  }

  function handleOAuth(provider: string) {
    window.location.href = `/auth/login?provider=${provider}`;
  }

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <div className="bg-white shadow-md rounded-lg p-8 w-full max-w-sm">
        <h1 className="text-xl font-semibold mb-6 text-center">LLM Router Admin</h1>

        {/* OAuth / SSO buttons */}
        {oauthProviders.length > 0 && (
          <div className="space-y-2 mb-4">
            {oauthProviders.map((p) => (
              <button
                key={p.name}
                type="button"
                onClick={() => handleOAuth(p.name)}
                className="w-full flex items-center justify-center gap-2 border border-gray-300 rounded px-4 py-2 text-sm font-medium hover:bg-gray-50 transition-colors"
              >
                <span className="w-5 text-center">{p.icon}</span>
                Continue with {p.label}
              </button>
            ))}
            <div className="flex items-center gap-2 my-4">
              <div className="flex-1 border-t border-gray-200" />
              <span className="text-xs text-gray-400">or use password</span>
              <div className="flex-1 border-t border-gray-200" />
            </div>
          </div>
        )}

        {/* Password form */}
        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label htmlFor="admin-password" className="block text-sm font-medium text-gray-700 mb-1">
              Password
            </label>
            <input
              id="admin-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Enter password"
              required
              autoComplete="current-password"
              className="w-full border border-gray-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>
          {error && (
            <p className="text-sm text-red-600">{error}</p>
          )}
          <button
            type="submit"
            disabled={loading}
            className="w-full bg-blue-600 text-white rounded px-4 py-2 text-sm font-medium hover:bg-blue-700 disabled:opacity-50"
          >
            {loading ? "Signing in…" : "Sign in"}
          </button>
        </form>
      </div>
    </div>
  );
}

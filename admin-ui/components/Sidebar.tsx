"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { clsx } from "clsx";

const nav = [
  { href: "/dashboard", label: "Dashboard" },
  { href: "/dashboard/keys", label: "Virtual Keys" },
  { href: "/dashboard/providers", label: "Providers" },
  { href: "/dashboard/usage", label: "Usage" },
  { href: "/dashboard/logs", label: "Logs" },
];

export default function Sidebar() {
  const pathname = usePathname();

  return (
    <aside className="w-56 min-h-screen bg-slate-900 text-slate-100 flex flex-col">
      <div className="px-6 py-5 border-b border-slate-700">
        <span className="font-bold text-lg tracking-tight">LLM Router</span>
        <p className="text-xs text-slate-400 mt-0.5">Admin Dashboard</p>
      </div>
      <nav className="flex-1 px-3 py-4 space-y-1">
        {nav.map(({ href, label }) => (
          <Link
            key={href}
            href={href}
            className={clsx(
              "flex items-center px-3 py-2 rounded-md text-sm font-medium transition-colors",
              pathname === href
                ? "bg-brand-600 text-white"
                : "text-slate-300 hover:bg-slate-700 hover:text-white"
            )}
          >
            {label}
          </Link>
        ))}
      </nav>
      <div className="px-4 py-3 border-t border-slate-700 text-xs text-slate-500">
        v1.0.0
      </div>
    </aside>
  );
}

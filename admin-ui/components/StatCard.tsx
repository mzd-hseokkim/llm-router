interface StatCardProps {
  title: string;
  value: string | number;
  sub?: string;
  colorClass?: string;
}

export default function StatCard({ title, value, sub, colorClass = "text-slate-900" }: StatCardProps) {
  return (
    <div className="bg-white rounded-xl border border-slate-200 p-5 shadow-sm">
      <p className="text-sm text-slate-500 font-medium">{title}</p>
      <p className={`mt-1 text-3xl font-bold ${colorClass}`}>{value}</p>
      {sub && <p className="mt-1 text-xs text-slate-400">{sub}</p>}
    </div>
  );
}

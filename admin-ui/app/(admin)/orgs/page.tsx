"use client";

import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { orgs, teams, users, Organization, Team, User } from "@/lib/api";

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

function Badge({ label, color }: { label: string; color: string }) {
  return (
    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium ${color}`}>
      {label}
    </span>
  );
}

function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: React.ReactNode }) {
  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50">
      <div className="bg-white rounded-xl shadow-xl w-full max-w-md p-6 space-y-4">
        <div className="flex items-center justify-between">
          <h2 className="text-lg font-semibold">{title}</h2>
          <button onClick={onClose} className="text-slate-400 hover:text-slate-600 text-xl leading-none">&times;</button>
        </div>
        {children}
      </div>
    </div>
  );
}

function ErrorMsg({ msg }: { msg: string }) {
  return msg ? <p className="text-sm text-red-600">{msg}</p> : null;
}

// ---------------------------------------------------------------------------
// Org section
// ---------------------------------------------------------------------------

function OrgDialog({ onClose, initial }: { onClose: () => void; initial?: Organization }) {
  const qc = useQueryClient();
  const [name, setName] = useState(initial?.name ?? "");
  const [error, setError] = useState("");

  const mutation = useMutation({
    mutationFn: () => initial ? orgs.update(initial.id, name) : orgs.create(name),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["orgs"] }); onClose(); },
    onError: (e: Error) => setError(e.message),
  });

  return (
    <Modal title={initial ? "Edit Organization" : "New Organization"} onClose={onClose}>
      <ErrorMsg msg={error} />
      <label className="block">
        <span className="text-sm font-medium text-slate-700">Name *</span>
        <input
          className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoFocus
        />
      </label>
      <div className="flex gap-3 pt-2">
        <button
          onClick={() => mutation.mutate()}
          disabled={!name.trim() || mutation.isPending}
          className="flex-1 bg-brand-600 text-white py-2 rounded-lg text-sm font-medium disabled:opacity-50"
        >
          {mutation.isPending ? "Saving…" : initial ? "Save" : "Create"}
        </button>
        <button onClick={onClose} className="flex-1 border border-slate-300 text-slate-700 py-2 rounded-lg text-sm font-medium">
          Cancel
        </button>
      </div>
    </Modal>
  );
}

// ---------------------------------------------------------------------------
// Team section
// ---------------------------------------------------------------------------

function TeamDialog({
  orgId,
  orgList,
  onClose,
  initial,
}: {
  orgId: string;
  orgList: Organization[];
  onClose: () => void;
  initial?: Team;
}) {
  const qc = useQueryClient();
  const [name, setName] = useState(initial?.name ?? "");
  const [selectedOrg, setSelectedOrg] = useState(initial?.org_id ?? orgId);
  const [error, setError] = useState("");

  const mutation = useMutation({
    mutationFn: () =>
      initial ? teams.update(initial.id, name) : teams.create(selectedOrg, name),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["teams", orgId] });
      onClose();
    },
    onError: (e: Error) => setError(e.message),
  });

  return (
    <Modal title={initial ? "Edit Team" : "New Team"} onClose={onClose}>
      <ErrorMsg msg={error} />
      {!initial && (
        <label className="block">
          <span className="text-sm font-medium text-slate-700">Organization *</span>
          <select
            className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
            value={selectedOrg}
            onChange={(e) => setSelectedOrg(e.target.value)}
          >
            {orgList.map((o) => (
              <option key={o.id} value={o.id}>{o.name}</option>
            ))}
          </select>
        </label>
      )}
      <label className="block">
        <span className="text-sm font-medium text-slate-700">Name *</span>
        <input
          className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
          value={name}
          onChange={(e) => setName(e.target.value)}
          autoFocus
        />
      </label>
      <div className="flex gap-3 pt-2">
        <button
          onClick={() => mutation.mutate()}
          disabled={!name.trim() || mutation.isPending}
          className="flex-1 bg-brand-600 text-white py-2 rounded-lg text-sm font-medium disabled:opacity-50"
        >
          {mutation.isPending ? "Saving…" : initial ? "Save" : "Create"}
        </button>
        <button onClick={onClose} className="flex-1 border border-slate-300 text-slate-700 py-2 rounded-lg text-sm font-medium">
          Cancel
        </button>
      </div>
    </Modal>
  );
}

// ---------------------------------------------------------------------------
// User section
// ---------------------------------------------------------------------------

function UserDialog({
  orgId,
  teamList,
  onClose,
  initial,
}: {
  orgId: string;
  teamList: Team[];
  onClose: () => void;
  initial?: User;
}) {
  const qc = useQueryClient();
  const [email, setEmail] = useState(initial?.email ?? "");
  const [teamId, setTeamId] = useState(initial?.team_id ?? "");
  const [error, setError] = useState("");

  const mutation = useMutation({
    mutationFn: () =>
      initial
        ? users.update(initial.id, email, teamId || undefined)
        : users.create(orgId, email, teamId || undefined),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["users", orgId] });
      onClose();
    },
    onError: (e: Error) => setError(e.message),
  });

  return (
    <Modal title={initial ? "Edit User" : "New User"} onClose={onClose}>
      <ErrorMsg msg={error} />
      <label className="block">
        <span className="text-sm font-medium text-slate-700">Email *</span>
        <input
          type="email"
          className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
          value={email}
          onChange={(e) => setEmail(e.target.value)}
          autoFocus
        />
      </label>
      <label className="block">
        <span className="text-sm font-medium text-slate-700">Team</span>
        <select
          className="mt-1 block w-full border border-slate-300 rounded-lg px-3 py-2 text-sm"
          value={teamId}
          onChange={(e) => setTeamId(e.target.value)}
        >
          <option value="">— No team —</option>
          {teamList.map((t) => (
            <option key={t.id} value={t.id}>{t.name}</option>
          ))}
        </select>
      </label>
      <div className="flex gap-3 pt-2">
        <button
          onClick={() => mutation.mutate()}
          disabled={!email.trim() || mutation.isPending}
          className="flex-1 bg-brand-600 text-white py-2 rounded-lg text-sm font-medium disabled:opacity-50"
        >
          {mutation.isPending ? "Saving…" : initial ? "Save" : "Invite"}
        </button>
        <button onClick={onClose} className="flex-1 border border-slate-300 text-slate-700 py-2 rounded-lg text-sm font-medium">
          Cancel
        </button>
      </div>
    </Modal>
  );
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------

export default function OrgsPage() {
  const qc = useQueryClient();

  // Selected org/team for drill-down
  const [selectedOrgId, setSelectedOrgId] = useState<string | null>(null);

  // Dialog state
  const [orgDialog, setOrgDialog] = useState<{ open: boolean; item?: Organization }>({ open: false });
  const [teamDialog, setTeamDialog] = useState<{ open: boolean; item?: Team }>({ open: false });
  const [userDialog, setUserDialog] = useState<{ open: boolean; item?: User }>({ open: false });

  // --- Data ---
  const { data: orgList = [], isLoading: orgsLoading } = useQuery({
    queryKey: ["orgs"],
    queryFn: () => orgs.list(),
  });

  const { data: teamList = [], isLoading: teamsLoading } = useQuery({
    queryKey: ["teams", selectedOrgId],
    queryFn: () => teams.list(selectedOrgId ?? undefined),
    enabled: !!selectedOrgId,
  });

  const { data: userList = [], isLoading: usersLoading } = useQuery({
    queryKey: ["users", selectedOrgId],
    queryFn: () => users.list(selectedOrgId ?? undefined),
    enabled: !!selectedOrgId,
  });

  const selectedOrg = orgList.find((o) => o.id === selectedOrgId);

  function fmt(date: string) {
    return new Date(date).toLocaleDateString();
  }

  function teamName(id?: string) {
    if (!id) return "—";
    return teamList.find((t) => t.id === id)?.name ?? id.slice(0, 8) + "…";
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-slate-900">Organizations</h1>
          <p className="text-sm text-slate-500 mt-1">Org → Team → User hierarchy</p>
        </div>
        <button
          onClick={() => setOrgDialog({ open: true })}
          className="bg-brand-600 text-white px-4 py-2 rounded-lg text-sm font-medium hover:bg-brand-700"
        >
          + New Org
        </button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6 items-start">
        {/* ---- Orgs panel ---- */}
        <div className="bg-white rounded-xl border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-4 py-3 bg-slate-50 border-b border-slate-200 flex items-center justify-between">
            <span className="text-xs font-semibold uppercase tracking-wide text-slate-500">
              Organizations ({orgList.length})
            </span>
          </div>
          {orgsLoading ? (
            <p className="text-sm text-slate-400 p-4">Loading…</p>
          ) : orgList.length === 0 ? (
            <p className="text-sm text-slate-400 p-4 text-center">No organizations yet.</p>
          ) : (
            <ul className="divide-y divide-slate-100">
              {orgList.map((org) => (
                <li
                  key={org.id}
                  onClick={() => setSelectedOrgId(org.id === selectedOrgId ? null : org.id)}
                  className={`px-4 py-3 cursor-pointer flex items-center justify-between hover:bg-slate-50 transition-colors ${
                    org.id === selectedOrgId ? "bg-brand-50 border-l-2 border-brand-600" : ""
                  }`}
                >
                  <div>
                    <p className="text-sm font-medium text-slate-900">{org.name}</p>
                    <p className="text-xs text-slate-400 mt-0.5">{fmt(org.created_at)}</p>
                  </div>
                  <button
                    onClick={(e) => { e.stopPropagation(); setOrgDialog({ open: true, item: org }); }}
                    className="text-xs text-slate-400 hover:text-brand-600"
                  >
                    Edit
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* ---- Teams panel ---- */}
        <div className="bg-white rounded-xl border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-4 py-3 bg-slate-50 border-b border-slate-200 flex items-center justify-between">
            <span className="text-xs font-semibold uppercase tracking-wide text-slate-500">
              {selectedOrg ? `Teams — ${selectedOrg.name}` : "Teams"}
            </span>
            {selectedOrgId && (
              <button
                onClick={() => setTeamDialog({ open: true })}
                className="text-xs text-brand-600 hover:underline font-medium"
              >
                + Add
              </button>
            )}
          </div>
          {!selectedOrgId ? (
            <p className="text-sm text-slate-400 p-4 text-center">Select an org to view teams.</p>
          ) : teamsLoading ? (
            <p className="text-sm text-slate-400 p-4">Loading…</p>
          ) : teamList.length === 0 ? (
            <p className="text-sm text-slate-400 p-4 text-center">No teams in this org.</p>
          ) : (
            <ul className="divide-y divide-slate-100">
              {teamList.map((team) => (
                <li key={team.id} className="px-4 py-3 flex items-center justify-between hover:bg-slate-50">
                  <div>
                    <p className="text-sm font-medium text-slate-900">{team.name}</p>
                    <p className="text-xs text-slate-400 mt-0.5">{fmt(team.created_at)}</p>
                  </div>
                  <button
                    onClick={() => setTeamDialog({ open: true, item: team })}
                    className="text-xs text-slate-400 hover:text-brand-600"
                  >
                    Edit
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        {/* ---- Users panel ---- */}
        <div className="bg-white rounded-xl border border-slate-200 shadow-sm overflow-hidden">
          <div className="px-4 py-3 bg-slate-50 border-b border-slate-200 flex items-center justify-between">
            <span className="text-xs font-semibold uppercase tracking-wide text-slate-500">
              {selectedOrg ? `Users — ${selectedOrg.name}` : "Users"}
            </span>
            {selectedOrgId && (
              <button
                onClick={() => setUserDialog({ open: true })}
                className="text-xs text-brand-600 hover:underline font-medium"
              >
                + Invite
              </button>
            )}
          </div>
          {!selectedOrgId ? (
            <p className="text-sm text-slate-400 p-4 text-center">Select an org to view users.</p>
          ) : usersLoading ? (
            <p className="text-sm text-slate-400 p-4">Loading…</p>
          ) : userList.length === 0 ? (
            <p className="text-sm text-slate-400 p-4 text-center">No users in this org.</p>
          ) : (
            <ul className="divide-y divide-slate-100">
              {userList.map((user) => (
                <li key={user.id} className="px-4 py-3 flex items-center justify-between hover:bg-slate-50">
                  <div>
                    <p className="text-sm font-medium text-slate-900">{user.email}</p>
                    <p className="text-xs text-slate-400 mt-0.5">
                      {teamName(user.team_id)}
                      <span className="mx-1">·</span>
                      {fmt(user.created_at)}
                    </p>
                  </div>
                  <button
                    onClick={() => setUserDialog({ open: true, item: user })}
                    className="text-xs text-slate-400 hover:text-brand-600"
                  >
                    Edit
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>

      {/* Dialogs */}
      {orgDialog.open && (
        <OrgDialog onClose={() => setOrgDialog({ open: false })} initial={orgDialog.item} />
      )}
      {teamDialog.open && selectedOrgId && (
        <TeamDialog
          orgId={selectedOrgId}
          orgList={orgList}
          onClose={() => setTeamDialog({ open: false })}
          initial={teamDialog.item}
        />
      )}
      {userDialog.open && selectedOrgId && (
        <UserDialog
          orgId={selectedOrgId}
          teamList={teamList}
          onClose={() => setUserDialog({ open: false })}
          initial={userDialog.item}
        />
      )}
    </div>
  );
}

import { NextRequest, NextResponse } from "next/server";

const SESSION_COOKIE = "admin_session";
const GATEWAY_URL = process.env.GATEWAY_URL ?? "http://localhost:8080";

export async function POST(request: NextRequest) {
  const { key } = await request.json().catch(() => ({ key: "" }));
  if (!key || typeof key !== "string") {
    return NextResponse.json({ error: "Key is required" }, { status: 400 });
  }

  // Validate the key against the gateway.
  const probe = await fetch(`${GATEWAY_URL}/admin/keys?limit=1`, {
    headers: { Authorization: `Bearer ${key}` },
    cache: "no-store",
  }).catch(() => null);

  if (!probe || probe.status === 401 || probe.status === 403) {
    return NextResponse.json({ error: "Invalid admin key" }, { status: 401 });
  }

  // Key is valid — set an httpOnly session cookie.
  const response = NextResponse.json({ ok: true });
  response.cookies.set(SESSION_COOKIE, key, {
    httpOnly: true,
    sameSite: "lax",
    path: "/",
    // Omit `secure` so it works over plain HTTP in local dev.
    // In production, set NEXT_PUBLIC_SECURE_COOKIE=true and enable it.
    secure: process.env.NODE_ENV === "production",
  });
  return response;
}

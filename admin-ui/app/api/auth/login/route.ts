import { NextRequest, NextResponse } from "next/server";

const SESSION_COOKIE = "admin_session";
const GATEWAY_URL = process.env.GATEWAY_URL ?? "http://localhost:8080";

export async function POST(request: NextRequest) {
  const { password } = await request.json().catch(() => ({ password: "" }));
  if (!password || typeof password !== "string") {
    return NextResponse.json({ error: "Password is required" }, { status: 400 });
  }

  const gatewayRes = await fetch(`${GATEWAY_URL}/admin/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ password }),
    cache: "no-store",
  }).catch(() => null);

  if (!gatewayRes || !gatewayRes.ok) {
    return NextResponse.json({ error: "Invalid credentials" }, { status: 401 });
  }

  const data = await gatewayRes.json().catch(() => null);
  if (!data?.token) {
    return NextResponse.json({ error: "Invalid credentials" }, { status: 401 });
  }

  const response = NextResponse.json({ ok: true, password_changed: data.password_changed });
  response.cookies.set(SESSION_COOKIE, data.token, {
    httpOnly: true,
    sameSite: "lax",
    path: "/",
    secure: process.env.NODE_ENV === "production",
  });
  return response;
}

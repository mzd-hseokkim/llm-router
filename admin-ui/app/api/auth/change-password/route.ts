import { NextRequest, NextResponse } from "next/server";

const SESSION_COOKIE = "admin_session";
const GATEWAY_URL = process.env.GATEWAY_URL ?? "http://localhost:8080";

export async function POST(request: NextRequest) {
  const token = request.cookies.get(SESSION_COOKIE)?.value;
  if (!token) {
    return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
  }

  const body = await request.json().catch(() => null);
  if (!body) {
    return NextResponse.json({ error: "Invalid request body" }, { status: 400 });
  }

  const gatewayRes = await fetch(`${GATEWAY_URL}/admin/auth/change-password`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
    },
    body: JSON.stringify(body),
    cache: "no-store",
  }).catch(() => null);

  if (!gatewayRes || !gatewayRes.ok) {
    const errData = await gatewayRes?.json().catch(() => null);
    return NextResponse.json(
      { error: errData?.error?.message ?? "Failed to change password" },
      { status: gatewayRes?.status ?? 500 }
    );
  }

  const data = await gatewayRes.json().catch(() => null);
  if (!data?.token) {
    return NextResponse.json({ error: "Unexpected response from gateway" }, { status: 500 });
  }

  const response = NextResponse.json({ ok: true });
  response.cookies.set(SESSION_COOKIE, data.token, {
    httpOnly: true,
    sameSite: "lax",
    path: "/",
    secure: process.env.NODE_ENV === "production",
  });
  return response;
}

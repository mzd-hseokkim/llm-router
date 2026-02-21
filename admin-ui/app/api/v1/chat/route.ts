import { NextRequest } from "next/server";

const GATEWAY = process.env.GATEWAY_URL || "http://localhost:8080";

export async function POST(req: NextRequest) {
  const auth = req.headers.get("authorization") || "";
  if (!auth) {
    return Response.json({ error: { message: "Authorization header required" } }, { status: 401 });
  }

  const body = await req.text();

  const upstream = await fetch(`${GATEWAY}/v1/chat/completions`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Authorization: auth,
    },
    body,
  });

  const contentType = upstream.headers.get("content-type") || "application/json";
  const headers: Record<string, string> = { "Content-Type": contentType };

  if (contentType.includes("text/event-stream")) {
    headers["Cache-Control"] = "no-cache";
    headers["X-Accel-Buffering"] = "no";
  }

  return new Response(upstream.body, { status: upstream.status, headers });
}

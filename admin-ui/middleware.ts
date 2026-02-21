import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

const SESSION_COOKIE = "admin_session";

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;
  const session = request.cookies.get(SESSION_COOKIE);

  // Protect /api/admin/* — inject Authorization header from httpOnly cookie
  // so the Next.js rewrite forwards it to the Go gateway.
  if (pathname.startsWith("/api/admin")) {
    if (!session?.value) {
      return NextResponse.json({ error: "Unauthorized" }, { status: 401 });
    }
    const requestHeaders = new Headers(request.headers);
    requestHeaders.set("Authorization", `Bearer ${session.value}`);
    return NextResponse.next({ request: { headers: requestHeaders } });
  }

  // Protect all admin pages — redirect to /login if not authenticated.
  if (!session?.value) {
    return NextResponse.redirect(new URL("/login", request.url));
  }

  return NextResponse.next();
}

export const config = {
  // Match all routes except /login, /api/auth/*, and Next.js internals.
  matcher: [
    "/((?!login|api/auth|_next/static|_next/image|favicon.ico).*)",
  ],
};

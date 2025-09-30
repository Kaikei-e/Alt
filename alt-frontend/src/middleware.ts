// src/middleware.ts
// Ory Kratos authentication middleware following official best practices
import { NextRequest, NextResponse } from "next/server";

const PUBLIC_ROUTES = [
  /^\/$/,
  /^\/auth(\/|$)/,
  /^\/api(\/|$)/,
  /^\/_next(\/|$)/,
  /^\/ory(\/|$)/,
  /^\/static(\/|$)/,
  /^\/favicon\.ico$/,
];

export async function middleware(req: NextRequest) {
  const { pathname, search } = req.nextUrl;

  // Allow public routes
  if (PUBLIC_ROUTES.some((pattern) => pattern.test(pathname))) {
    return NextResponse.next();
  }

  // Test environment: skip auth for test routes
  if (process.env.NODE_ENV === "test" && pathname.startsWith("/test/")) {
    return NextResponse.next();
  }

  // Fast path: Check for Kratos session cookie (Ory best practice)
  // If cookie exists, assume valid session (backend will validate)
  const sessionCookie = req.cookies.get("ory_kratos_session");
  if (sessionCookie?.value) {
    return NextResponse.next();
  }

  // No session cookie: redirect to login
  // Prevent redirect loop: if already coming from login flow, allow through
  const referer = req.headers.get("referer");
  if (referer && (referer.includes("/auth/login") || referer.includes("/ory/self-service/login"))) {
    return NextResponse.next();
  }

  // Following Ory pattern: direct redirect to Kratos login flow
  const appOrigin =
    process.env.NEXT_PUBLIC_APP_ORIGIN || req.nextUrl.origin;
  const kratosUrl =
    process.env.NEXT_PUBLIC_KRATOS_PUBLIC_URL || `${appOrigin}/ory`;

  // Build return URL, but validate it's from the same origin to prevent open redirect
  const returnTo = `${appOrigin}${pathname}${search}`;
  const loginFlowUrl = `${kratosUrl}/self-service/login/browser?return_to=${encodeURIComponent(returnTo)}`;

  return NextResponse.redirect(loginFlowUrl, 303);
}

export const config = {
  // Match all routes except static files
  matcher: ["/((?!.*\\.).*)"],
};

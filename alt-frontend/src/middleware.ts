// src/middleware.ts
// Ory Kratos authentication middleware following official best practices
import { type NextRequest, NextResponse } from "next/server";

const PUBLIC_ROUTES = [
  /^\/auth(\/|$)/,
  /^\/api(\/|$)/,
  /^\/_next(\/|$)/,
  /^\/ory(\/|$)/,
  /^\/static(\/|$)/,
  /^\/favicon\.ico$/,
  /^\/icon\.svg$/,
  /^\/test(\/|$)/, // Allow test pages for E2E testing
  /^\/public\/landing(\/|$)/, // Public landing page
  /^\/landing$/, // Landing page shortcut
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

  // Check for Kratos session cookie (fast path)
  const sessionCookie = req.cookies.get("ory_kratos_session");

  // Also check for cookie with underscore variant (Kratos may set both)
  const altSessionCookie = req.cookies.get("ory-kratos-session");

  const effectiveSessionCookie = sessionCookie || altSessionCookie;

  if (!effectiveSessionCookie?.value) {
    // No cookie: redirect to landing page with return URL
    const appOrigin = process.env.NEXT_PUBLIC_APP_ORIGIN || req.nextUrl.origin;
    const returnUrl = encodeURIComponent(`${pathname}${search}`);
    const landingUrl = `${appOrigin}/public/landing?return_to=${returnUrl}`;

    return NextResponse.redirect(landingUrl, 303);
  }

  // Validate session with auth-hub (uses 5min cache)
  try {
    const authHubUrl = process.env.AUTH_HUB_INTERNAL_URL || "http://auth-hub:8888";

    // Pass the actual cookie value, preserving the original cookie name
    const cookieHeader = `ory_kratos_session=${effectiveSessionCookie.value}`;

    const response = await fetch(`${authHubUrl}/session`, {
      headers: {
        cookie: cookieHeader,
        "x-forwarded-proto": "https",
        "x-forwarded-host": req.headers.get("host") || "",
      },
      cache: "no-store",
      // Add timeout to prevent hanging
      signal: AbortSignal.timeout(5000),
    });

    if (response.ok) {
      // Session is valid
      return NextResponse.next();
    }

    // Session invalid or expired: redirect to landing
    const appOrigin = process.env.NEXT_PUBLIC_APP_ORIGIN || req.nextUrl.origin;
    const returnUrl = encodeURIComponent(`${pathname}${search}`);
    const landingUrl = `${appOrigin}/public/landing?return_to=${returnUrl}`;
    return NextResponse.redirect(landingUrl, 303);
  } catch (error) {
    console.error("[Middleware] Session validation error:", error);
    // On error, redirect to landing (fail closed for security)
    const appOrigin = process.env.NEXT_PUBLIC_APP_ORIGIN || req.nextUrl.origin;
    const returnUrl = encodeURIComponent(`${pathname}${search}`);
    const landingUrl = `${appOrigin}/public/landing?return_to=${returnUrl}`;
    return NextResponse.redirect(landingUrl, 303);
  }
}

export const config = {
  // Match all routes except static files
  matcher: ["/((?!.*\\.).*)"],
};

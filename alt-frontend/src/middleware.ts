// src/middleware.ts
import { NextRequest, NextResponse } from "next/server";

const PUBLIC = [
  /^\/$/,
  /^\/auth(\/|$)/,
  /^\/api(\/|$)/,
  /^\/_next(\/|$)/,
  /^\/static(\/|$)/,
  /^\/favicon\.ico$/,
];

export async function middleware(req: NextRequest) {
  // In test environment, still run auth logic but use test URLs
  const isTest = process.env.NODE_ENV === "test";

  const { pathname, search } = req.nextUrl;
  if (PUBLIC.some((r) => r.test(pathname))) return NextResponse.next();

  // Skip auth check for test component routes
  if (isTest && pathname.startsWith("/test/")) return NextResponse.next();

  // Fast-path cookie check
  if (req.cookies.get("ory_kratos_session")) return NextResponse.next();

  // Prevent multi-redirect loops: removed unconditional bypass cookie to avoid unauthorized prefetch passing through

  // Network fallback: verify via FE route handler to avoid proxy quirks
  try {
    const validateUrl = new URL("/api/fe-auth/validate", req.url);
    const cookieHeader = req.headers.get("cookie") || "";
    const r = await fetch(validateUrl, {
      method: "GET",
      headers: { cookie: cookieHeader },
      cache: "no-store",
    });
    if (r.ok) {
      return NextResponse.next();
    }
  } catch {
    // ignore and fall through
  }

  // 未ログイン時はアプリ内のログインUIへ
  const appOrigin = process.env.NEXT_PUBLIC_APP_ORIGIN || req.nextUrl.origin;
  const loginPage = new URL(`/auth/login`, appOrigin);
  loginPage.searchParams.set("return_to", `${appOrigin}${pathname}${search}`);

  return NextResponse.redirect(loginPage, 303);
}

export const config = {
  matcher: ["/((?!.*\\.).*)"],
};

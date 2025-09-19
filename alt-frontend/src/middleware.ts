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

export function middleware(req: NextRequest) {
  // In test environment, still run auth logic but use test URLs
  const isTest = process.env.NODE_ENV === "test";

  const { pathname, search } = req.nextUrl;
  if (PUBLIC.some((r) => r.test(pathname))) return NextResponse.next();

  // Skip auth check for test component routes
  if (isTest && pathname.startsWith("/test/")) return NextResponse.next();

  // Edge Cookie check
  if (req.cookies.get("ory_kratos_session")) return NextResponse.next();

  // Multi-redirect prevention
  if (req.cookies.get("alt_auth_redirect_guard")) return NextResponse.next();

  // Use test auth server in test mode
  // 未ログイン時はアプリ内のログインUIへ
  const appOrigin = process.env.NEXT_PUBLIC_APP_ORIGIN || req.nextUrl.origin;
  const loginPage = new URL(`/auth/login`, appOrigin);
  loginPage.searchParams.set("return_to", `${appOrigin}${pathname}${search}`);

  const res = NextResponse.redirect(loginPage, 303);
  res.cookies.set("alt_auth_redirect_guard", "1", {
    maxAge: 10,
    httpOnly: true,
    secure: !isTest,
    sameSite: "lax",
    path: "/",
  });
  return res;
}

export const config = {
  matcher: ["/((?!.*\\.).*)"],
};

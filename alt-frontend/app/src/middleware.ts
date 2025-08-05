import { NextRequest, NextResponse } from "next/server";
import { securityHeaders } from "./config/security";

export function middleware(request: NextRequest) {
  const { pathname } = request.nextUrl;

  // 静的ファイルの明示的な除外処理
  if (pathname === '/manifest.json') {
    return NextResponse.next();
  }

  const nonce = Buffer.from(crypto.randomUUID()).toString("base64");
  const headers = new Headers(request.headers);

  const cspHeader = securityHeaders(nonce)["Content-Security-Policy"];

  headers.set("Content-Security-Policy", cspHeader);
  headers.set("x-nonce", nonce);

  const response = NextResponse.next({
    request: {
      headers: headers,
    },
  });

  // レスポンスヘッダーにもCSPを設定
  for (const [key, value] of Object.entries(securityHeaders(nonce))) {
    response.headers.set(key, value);
  }

  return response;
}

export const config = {
  matcher: [
    {
      source: "/((?!api|_next/static|_next/image|favicon.ico|.*\\.png|.*\\.ico|.*\\.svg).*)",
      missing: [
        { type: "header", key: "next-router-prefetch" },
        { type: "header", key: "purpose", value: "prefetch" },
      ],
    },
  ],
};
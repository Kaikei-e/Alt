// Debug endpoint for cookie visibility - TODO.md要件

import { cookies } from "next/headers";
import { type NextRequest, NextResponse } from "next/server";

export const dynamic = "force-dynamic";
export const revalidate = 0;

export async function GET(request: NextRequest) {
  try {
    // TODO.md要件: サーバが受信したCookieの詳細をデバッグ用に可視化
    const cookieStore = await cookies();

    const sessionCookies = {
      ory_kratos_session: cookieStore.has("ory_kratos_session"),
      host_ory_kratos_session: cookieStore.has("__Host-ory_kratos_session"),
    };

    const hasAnySession =
      sessionCookies.ory_kratos_session ||
      sessionCookies.host_ory_kratos_session;

    // Get all cookie names for debugging
    const allCookieNames = Array.from(cookieStore.getAll()).map((cookie) => ({
      name: cookie.name,
      hasValue: !!cookie.value,
      valueLength: cookie.value?.length || 0,
    }));

    const debugInfo = {
      timestamp: new Date().toISOString(),
      hasSession: hasAnySession,
      sessionCookies,
      allCookies: allCookieNames,
      cookieCount: allCookieNames.length,
      userAgent: request.headers.get("user-agent"),
      origin: request.headers.get("origin"),
      referer: request.headers.get("referer"),
      host: request.headers.get("host"),
    };

    return NextResponse.json(debugInfo, {
      status: 200,
      headers: {
        "Content-Type": "application/json",
        "Cache-Control": "no-cache, no-store, must-revalidate",
        Pragma: "no-cache",
        Expires: "0",
        // TODO.md要件: X-Has-Session ヘッダで即座に可視化
        "X-Has-Session": hasAnySession ? "yes" : "no",
        "X-Cookie-Count": allCookieNames.length.toString(),
        "X-Debug-Endpoint": "cookie-visibility",
      },
    });
  } catch (error) {
    console.error("Cookie debug endpoint error:", error);
    return NextResponse.json(
      {
        error: "Cookie debug failed",
        timestamp: new Date().toISOString(),
      },
      {
        status: 500,
        headers: {
          "Content-Type": "application/json",
          "X-Has-Session": "error",
        },
      },
    );
  }
}

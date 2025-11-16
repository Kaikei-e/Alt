import { cookies } from "next/headers";
import { type NextRequest, NextResponse } from "next/server";

export async function GET(_request: NextRequest) {
  try {
    // TODO.md要件: サーバが受信したCookieの有無をデバッグ用に可視化
    const cookieStore = await cookies();
    const hasOryKratosSession = await cookieStore.has("ory_kratos_session");
    const hasHostOryKratosSession = await cookieStore.has(
      "__Host-ory_kratos_session",
    );
    const hasKratosSession = hasOryKratosSession || hasHostOryKratosSession;

    // Basic application health checks
    // Dockerfile の HEALTHCHECK で "\"status\":\"ok\"" を grep しているため、
    // 必ず status は "ok" を返す
    return NextResponse.json(
      {
        status: "ok",
        timestamp: new Date().toISOString(),
        environment: process.env.NODE_ENV || "unknown",
        hasSession: hasKratosSession,
      },
      {
        status: 200,
        headers: {
          "Content-Type": "application/json",
          "Cache-Control": "no-cache, no-store, must-revalidate",
          "X-Health-Check": "alt-frontend-2025",
          // TODO.md要件: X-Has-Session ヘッダで即座に可視化
          "X-Has-Session": hasKratosSession ? "yes" : "no",
        },
      },
    );
  } catch (_error) {
    return NextResponse.json(
      {
        status: "ERROR",
        error: "Health check failed",
        timestamp: new Date().toISOString(),
      },
      {
        status: 503,
        headers: {
          "Content-Type": "application/json",
          "X-Health-Check-Error": "true",
        },
      },
    );
  }
}

// 2025年ベストプラクティス: HEAD requestもサポート
export async function HEAD(_request: NextRequest) {
  return new NextResponse(null, {
    status: 200,
    headers: {
      "X-Health-Check": "alt-frontend-2025",
    },
  });
}

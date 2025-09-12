import { NextRequest, NextResponse } from "next/server";
import { cookies } from "next/headers";

export async function GET(request: NextRequest) {
  try {
    // TODO.md要件: サーバが受信したCookieの有無をデバッグ用に可視化
    const cookieStore = await cookies();
    const hasOryKratosSession = cookieStore.has("ory_kratos_session");
    const hasHostOryKratosSession = cookieStore.has(
      "__Host-ory_kratos_session",
    );
    const hasKratosSession = hasOryKratosSession || hasHostOryKratosSession;

    // Basic application health checks
    const healthCheck = {
      status: "OK",
      timestamp: new Date().toISOString(),
      uptime: process.uptime(),
      environment: process.env.NODE_ENV || "unknown",
      version: process.env.npm_package_version || "1.0.0",
      checks: {
        memory: {
          used: Math.round(process.memoryUsage().heapUsed / 1024 / 1024),
          total: Math.round(process.memoryUsage().heapTotal / 1024 / 1024),
          status: "OK",
        },
        server: {
          status: "OK",
        },
        // TODO.md要件: Cookie受信状況をレスポンスに含める
        authentication: {
          hasSession: hasKratosSession,
          cookies: {
            ory_kratos_session: hasOryKratosSession,
            host_ory_kratos_session: hasHostOryKratosSession,
          },
        },
      },
    };

    return NextResponse.json(healthCheck, {
      status: 200,
      headers: {
        "Content-Type": "application/json",
        "Cache-Control": "no-cache, no-store, must-revalidate",
        "X-Health-Check": "alt-frontend-2025",
        // TODO.md要件: X-Has-Session ヘッダで即座に可視化
        "X-Has-Session": hasKratosSession ? "yes" : "no",
      },
    });
  } catch (error) {
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
export async function HEAD(request: NextRequest) {
  return new NextResponse(null, {
    status: 200,
    headers: {
      "X-Health-Check": "alt-frontend-2025",
    },
  });
}

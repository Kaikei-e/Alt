import { NextRequest, NextResponse } from "next/server";

export async function POST(request: NextRequest) {
  try {
    const report = await request.json();

    // CSP違反をログに記録

    // 本番環境では外部監視サービスに送信
    if (process.env.NODE_ENV === "production") {
      // await sendToMonitoringService(report);
    }

    return new NextResponse(null, { status: 204 });
  } catch (error) {
    console.error("Error processing CSP report:", error);
    return NextResponse.json(
      { error: "Internal server error" },
      { status: 500 },
    );
  }
}

// CSP report endpoint は POST のみ許可
export async function GET() {
  return NextResponse.json({ error: "Method not allowed" }, { status: 405 });
}

import { NextRequest, NextResponse } from "next/server";

export const dynamic = "force-dynamic";

export async function GET(req: NextRequest) {
  const requestId = crypto.randomUUID();
  console.log(`[API] Auth validate start [${requestId}]`);

  try {
    const authHubUrl =
      process.env.AUTH_HUB_INTERNAL_URL || "http://auth-hub:8888";

    // Pass all cookies to auth-hub
    const cookieHeader = req.headers.get("cookie") || "";
    console.log(`[API] [${requestId}] Cookies received length: ${cookieHeader.length}`);
    if (cookieHeader.length === 0) {
      console.warn(`[API] [${requestId}] NO COOKIES RECEIVED!`);
    } else {
      // Log cookie names only for security
      const cookieNames = cookieHeader.split(';').map(c => c.split('=')[0].trim());
      console.log(`[API] [${requestId}] Cookie names: ${cookieNames.join(', ')}`);
    }

    console.log(`[API] [${requestId}] Fetching ${authHubUrl}/session`);
    const response = await fetch(`${authHubUrl}/session`, {
      headers: {
        cookie: cookieHeader,
        "x-forwarded-proto": req.headers.get("x-forwarded-proto") || "https",
        "x-forwarded-host": req.headers.get("host") || "",
      },
      cache: "no-store",
      signal: AbortSignal.timeout(5000),
    });

    console.log(`[API] [${requestId}] Auth-hub response status: ${response.status}`);

    if (response.ok) {
      const data = await response.json();
      console.log(`[API] [${requestId}] Auth-hub success`);
      return NextResponse.json(data);
    }

    // If auth-hub returns 401 or other error, forward it
    console.warn(`[API] [${requestId}] Auth-hub failed: ${response.status}`);

    // Try to read error body if possible
    try {
      const errorText = await response.text();
      console.warn(`[API] [${requestId}] Auth-hub error body: ${errorText.substring(0, 200)}`);
    } catch (e) {
      console.warn(`[API] [${requestId}] Could not read error body`);
    }

    return NextResponse.json(
      { valid: false },
      { status: response.status }
    );
  } catch (error) {
    console.error(`[API] [${requestId}] Auth validate error:`, error);
    return NextResponse.json(
      { valid: false, error: "Internal Server Error" },
      { status: 500 }
    );
  }
}

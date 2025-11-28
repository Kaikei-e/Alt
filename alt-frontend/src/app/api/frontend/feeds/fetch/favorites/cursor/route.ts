/**
 * API Route: /api/frontend/feeds/fetch/favorites/cursor
 * Proxies requests to alt-backend with JWT authentication
 */

import { type NextRequest, NextResponse } from "next/server";
import { getServerSessionHeaders } from "@/lib/auth/server-headers";

export const dynamic = "force-dynamic";

export async function GET(request: NextRequest) {
  try {
    const authHeaders = await getServerSessionHeaders();
    if (!authHeaders) {
      return NextResponse.json(
        { error: "Authentication required" },
        { status: 401 },
      );
    }

    const searchParams = request.nextUrl.searchParams;
    const limit = searchParams.get("limit");
    const cursor = searchParams.get("cursor");

    const queryParams = new URLSearchParams();
    if (limit) queryParams.set("limit", limit);
    if (cursor) queryParams.set("cursor", cursor);
    const queryString = queryParams.toString();

    const backendUrl = process.env.API_URL || "http://alt-backend:9000";
    const backendEndpoint = `${backendUrl}/v1/feeds/fetch/favorites/cursor${queryString ? `?${queryString}` : ""}`;

    const cookieHeader = request.headers.get("cookie") || "";
    const forwardedFor = request.headers.get("x-forwarded-for") || "";
    const forwardedProto = request.headers.get("x-forwarded-proto") || "https";

    const backendResponse = await fetch(backendEndpoint, {
      method: "GET",
      headers: {
        Cookie: cookieHeader,
        "Content-Type": "application/json",
        "X-Forwarded-For": forwardedFor,
        "X-Forwarded-Proto": forwardedProto,
        ...authHeaders,
      },
      cache: "no-store",
      signal: AbortSignal.timeout(30000),
    });

    if (!backendResponse.ok) {
      return NextResponse.json(
        { error: `Backend API error: ${backendResponse.status}` },
        { status: backendResponse.status },
      );
    }

    const data = await backendResponse.json();
    return NextResponse.json(data);
  } catch (error) {
    console.error("Error in /api/frontend/feeds/fetch/favorites/cursor:", error);

    if (error instanceof Error && error.name === "AbortError") {
      return NextResponse.json(
        { error: "Request timeout" },
        { status: 504 },
      );
    }

    return NextResponse.json(
      { error: "Internal server error" },
      { status: 500 },
    );
  }
}


/**
 * API Route: /api/articles/content
 * Proxies requests to alt-backend and sanitizes HTML content server-side
 */

import { NextRequest, NextResponse } from "next/server";
import { sanitizeForArticle } from "@/lib/server/sanitize-html";
import type { SafeHtmlString } from "@/lib/server/sanitize-html";

interface BackendResponse {
  content: string;
}

interface SafeResponse {
  content: SafeHtmlString;
}

export async function GET(request: NextRequest) {
  try {
    const searchParams = request.nextUrl.searchParams;
    const url = searchParams.get("url");

    if (!url) {
      return NextResponse.json(
        { error: "Missing required parameter: url" },
        { status: 400 },
      );
    }

    // Fetch from alt-backend
    const backendUrl = process.env.API_URL || "http://localhost:9000";
    const encodedUrl = encodeURIComponent(url);
    const backendEndpoint = `${backendUrl}/v1/articles/fetch/content?url=${encodedUrl}`;

    // Forward cookies and headers
    const cookieHeader = request.headers.get("cookie") || "";
    const forwardedFor = request.headers.get("x-forwarded-for") || "";
    const forwardedProto = request.headers.get("x-forwarded-proto") || "https";

    const backendResponse = await fetch(backendEndpoint, {
      headers: {
        Cookie: cookieHeader,
        "Content-Type": "application/json",
        "X-Forwarded-For": forwardedFor,
        "X-Forwarded-Proto": forwardedProto,
      },
      cache: "no-store",
      // Use shorter timeout (15 seconds) to prevent UI freezing
      signal: AbortSignal.timeout(15000),
    });

    if (!backendResponse.ok) {
      return NextResponse.json(
        { error: `Backend API error: ${backendResponse.status}` },
        { status: backendResponse.status },
      );
    }

    const backendData: BackendResponse = await backendResponse.json();

    // Sanitize HTML content server-side
    const sanitizedContent = sanitizeForArticle(backendData.content);

    const safeResponse: SafeResponse = {
      content: sanitizedContent,
    };

    return NextResponse.json(safeResponse);
  } catch (error) {
    console.error("Error in /api/articles/content:", error);

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


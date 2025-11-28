/**
 * API Route: /api/frontend/articles/content
 * Proxies requests to alt-backend and sanitizes HTML content server-side
 * POST method with SSRF protection
 */

import { type NextRequest, NextResponse } from "next/server";
import { sanitizeForArticle } from "@/lib/server/sanitize-html";
import { validateUrlForSSRF } from "@/lib/server/ssrf-validator";
import type { SafeHtmlString } from "@/lib/server/sanitize-html";
import { getServerSessionHeaders } from "@/lib/auth/server-headers";

interface RequestBody {
  url: string;
}

interface BackendResponse {
  content: string;
}

interface SafeResponse {
  content: SafeHtmlString;
}

// Handle POST requests
export async function POST(request: NextRequest) {
  try {
    const body: RequestBody = await request.json();

    if (!body.url || typeof body.url !== "string") {
      return NextResponse.json(
        { error: "Missing or invalid url parameter" },
        { status: 400 },
      );
    }

    // SSRF protection: validate URL before processing
    try {
      validateUrlForSSRF(body.url);
    } catch (error) {
      if (error instanceof Error && error.name === "SSRFValidationError") {
        return NextResponse.json(
          { error: "Invalid URL: SSRF protection blocked this request" },
          { status: 400 },
        );
      }
      throw error;
    }

    // Get authentication headers from session
    const authHeaders = await getServerSessionHeaders();
    if (!authHeaders) {
      return NextResponse.json(
        { error: "Authentication required" },
        { status: 401 },
      );
    }

    // Fetch from alt-backend
    const backendUrl = process.env.API_URL || "http://localhost:9000";
    const encodedUrl = encodeURIComponent(body.url);
    const backendEndpoint = `${backendUrl}/v1/articles/fetch/content?url=${encodedUrl}`;

    // Forward cookies and headers
    const cookieHeader = request.headers.get("cookie") || "";
    const forwardedFor = request.headers.get("x-forwarded-for") || "";
    const forwardedProto = request.headers.get("x-forwarded-proto") || "https";

    const backendResponse = await fetch(backendEndpoint, {
      method: "GET", // Backend endpoint only supports GET
      headers: {
        Cookie: cookieHeader,
        "Content-Type": "application/json",
        "X-Forwarded-For": forwardedFor,
        "X-Forwarded-Proto": forwardedProto,
        // Add authentication headers required by backend
        ...authHeaders,
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
    console.error("Error in /api/frontend/articles/content:", error);

    if (error instanceof Error && error.name === "AbortError") {
      return NextResponse.json({ error: "Request timeout" }, { status: 504 });
    }

    return NextResponse.json(
      { error: "Internal server error" },
      { status: 500 },
    );
  }
}

// Handle unsupported HTTP methods
export async function GET() {
  return NextResponse.json(
    { error: "Method not allowed. Use POST instead." },
    { status: 405 },
  );
}

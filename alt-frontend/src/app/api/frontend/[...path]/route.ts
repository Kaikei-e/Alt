/**
 * API Route: /api/frontend/[...path]
 * Generic proxy route for all backend API endpoints with JWT authentication
 * This implements the BFF pattern where all client requests go through Next.js API routes
 */

import { type NextRequest, NextResponse } from "next/server";
import { getServerSessionHeaders } from "@/lib/auth/server-headers";

export const dynamic = "force-dynamic";

// Allowed HTTP methods
const ALLOWED_METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE"];

export async function GET(
  request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const resolvedParams = await params;
  return handleRequest(request, resolvedParams, "GET");
}

export async function POST(
  request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const resolvedParams = await params;
  return handleRequest(request, resolvedParams, "POST");
}

export async function PUT(
  request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const resolvedParams = await params;
  return handleRequest(request, resolvedParams, "PUT");
}

export async function PATCH(
  request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const resolvedParams = await params;
  return handleRequest(request, resolvedParams, "PATCH");
}

export async function DELETE(
  request: NextRequest,
  { params }: { params: Promise<{ path: string[] }> },
) {
  const resolvedParams = await params;
  return handleRequest(request, resolvedParams, "DELETE");
}

async function handleRequest(
  request: NextRequest,
  params: { path: string[] },
  method: string,
) {
  try {
    // Get authentication headers from session (includes JWT token)
    const authHeaders = await getServerSessionHeaders();
    if (!authHeaders) {
      return NextResponse.json(
        { error: "Authentication required" },
        { status: 401 },
      );
    }

    // Build backend path from route params
    // Flow: Client -> /api/frontend/v1/feeds/... -> Nginx -> Next.js API Route -> alt-backend/v1/feeds/...
    // params.path contains everything after /api/frontend/
    // Example: /api/frontend/v1/feeds/fetch/cursor -> params.path = ["v1", "feeds", "fetch", "cursor"]
    // Example: /api/frontend/v2/articles/search -> params.path = ["v2", "articles", "search"]
    const pathSegments = params.path || [];

    // Join path segments and add leading slash
    // This preserves version prefixes (v1, v2, etc.) and all path components
    const backendPath = `/${pathSegments.join("/")}`;

    // Get query parameters
    const searchParams = request.nextUrl.searchParams;
    const queryString = searchParams.toString();

    // Build backend URL
    const backendUrl = process.env.API_URL || "http://alt-backend:9000";
    const backendEndpoint = `${backendUrl}${backendPath}${queryString ? `?${queryString}` : ""}`;

    // Forward cookies and headers
    const cookieHeader = request.headers.get("cookie") || "";
    const forwardedFor = request.headers.get("x-forwarded-for") || "";
    const forwardedProto = request.headers.get("x-forwarded-proto") || "https";

    // Prepare request body for POST/PUT/PATCH
    let body: string | undefined;
    if (["POST", "PUT", "PATCH"].includes(method)) {
      try {
        const requestBody = await request.json();
        body = JSON.stringify(requestBody);
      } catch {
        // No body or invalid JSON, continue without body
      }
    }

    const backendResponse = await fetch(backendEndpoint, {
      method,
      headers: {
        Cookie: cookieHeader,
        "Content-Type": "application/json",
        "X-Forwarded-For": forwardedFor,
        "X-Forwarded-Proto": forwardedProto,
        // Add authentication headers with JWT token
        ...authHeaders,
      },
      body,
      cache: "no-store",
      signal: AbortSignal.timeout(120000), // 2 minutes timeout
    });

    // Forward response status and headers
    const responseHeaders = new Headers();

    // Copy relevant headers from backend response
    const contentType = backendResponse.headers.get("content-type");
    if (contentType) {
      responseHeaders.set("content-type", contentType);
    }

    // Get response body
    const responseText = await backendResponse.text();

    // Return response with same status code
    return new NextResponse(responseText, {
      status: backendResponse.status,
      statusText: backendResponse.statusText,
      headers: responseHeaders,
    });
  } catch (error) {
    console.error(`Error in /api/frontend/${params.path?.join("/")}:`, error);

    if (error instanceof Error && error.name === "AbortError") {
      return NextResponse.json({ error: "Request timeout" }, { status: 504 });
    }

    return NextResponse.json(
      { error: "Internal server error" },
      { status: 500 },
    );
  }
}

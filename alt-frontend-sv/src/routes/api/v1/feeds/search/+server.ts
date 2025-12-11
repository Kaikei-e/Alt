import { json, type RequestHandler } from "@sveltejs/kit";
import { getBackendToken } from "$lib/api";
import { env } from "$env/dynamic/private";

const BACKEND_BASE_URL = env.BACKEND_BASE_URL || "http://alt-backend:9000";

export const POST: RequestHandler = async ({ request, url }) => {
  const cookieHeader = request.headers.get("cookie") || "";
  const token = await getBackendToken(cookieHeader);

  // Get query parameters for cursor-based pagination
  const queryParam = url.searchParams.get("query");
  const cursorParam = url.searchParams.get("cursor");
  const limitParam = url.searchParams.get("limit");

  // Parse request body
  let body: { query: string; cursor?: string; limit?: number };
  try {
    body = await request.json();
  } catch (error) {
    return json({ error: "Invalid JSON body" }, { status: 400 });
  }

  // Validate query
  if (!body.query || typeof body.query !== "string") {
    return json({ error: "Query is required" }, { status: 400 });
  }

  // Build backend endpoint
  const backendEndpoint = `${BACKEND_BASE_URL}/v1/feeds/search`;

  // Prepare request payload
  const payload: { query: string; cursor?: number; limit?: number } = {
    query: queryParam || body.query,
  };

  // Add cursor (offset) and limit if provided
  if (cursorParam) {
    const cursorOffset = parseInt(cursorParam, 10);
    if (!isNaN(cursorOffset)) {
      payload.cursor = cursorOffset;
    }
  } else if (body.cursor !== undefined && body.cursor !== null) {
    // Handle both string and number cursor from body
    if (typeof body.cursor === "number") {
      payload.cursor = body.cursor;
    } else if (typeof body.cursor === "string") {
      const cursorOffset = parseInt(body.cursor, 10);
      if (!isNaN(cursorOffset)) {
        payload.cursor = cursorOffset;
      }
    }
  }

  if (limitParam) {
    const limit = parseInt(limitParam, 10);
    if (!isNaN(limit)) {
      payload.limit = limit;
    }
  } else if (body.limit !== undefined && body.limit !== null) {
    payload.limit =
      typeof body.limit === "number" ? body.limit : parseInt(String(body.limit), 10);
  }

  try {
    const headers: HeadersInit = {
      "Content-Type": "application/json",
    };

    if (token) {
      headers["X-Alt-Backend-Token"] = token;
    }

    const response = await fetch(backendEndpoint, {
      method: "POST",
      headers,
      body: JSON.stringify(payload),
      cache: "no-store",
    });

    if (!response.ok) {
      const errorText = await response.text().catch(() => "");
      console.error("Backend API error:", {
        status: response.status,
        statusText: response.statusText,
        errorBody: errorText.substring(0, 200),
      });
      return json(
        { error: `Backend API error: ${response.status}` },
        { status: response.status },
      );
    }

    const data = await response.json();

    // Handle different response formats
    // If backend returns array directly, wrap it
    if (Array.isArray(data)) {
      return json({
        results: data,
        error: null,
        next_cursor: null,
        has_more: false,
      });
    }

    // If backend returns cursor-based response
    if (data.data && Array.isArray(data.data)) {
      // Convert next_cursor to number if it's a number
      let nextCursor: number | null = null;
      if (data.next_cursor !== null && data.next_cursor !== undefined) {
        if (typeof data.next_cursor === "number") {
          nextCursor = data.next_cursor;
        } else if (typeof data.next_cursor === "string") {
          const parsed = parseInt(data.next_cursor, 10);
          if (!isNaN(parsed)) {
            nextCursor = parsed;
          }
        }
      }
      return json({
        results: data.data,
        error: null,
        next_cursor: nextCursor,
        has_more: data.has_more ?? nextCursor !== null,
      });
    }

    // If backend returns FeedSearchResult format
    if (data.results && Array.isArray(data.results)) {
      return json({
        results: data.results,
        error: data.error || null,
        next_cursor: data.next_cursor || null,
        has_more: data.has_more ?? false,
      });
    }

    // Default: return as-is
    return json(data);
  } catch (error) {
    console.error("Error in /api/v1/feeds/search:", error);
    return json({ error: "Internal server error" }, { status: 500 });
  }
};


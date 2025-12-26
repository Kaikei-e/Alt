import { json, type RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { getBackendToken } from "$lib/api";
import { validateUrlForSSRF } from "$lib/server/ssrf-validator";

interface RequestBody {
  feed_url?: string;
  article_id?: string;
  content?: string;
  title?: string;
}

export const POST: RequestHandler = async ({ request, locals }) => {
  const startTime = Date.now();
  console.log("[StreamSummarize] Request received", {
    url: request.url,
    method: request.method,
    hasSession: !!locals.session,
  });

  // Early authentication check - fail fast before processing request body
  // This prevents unnecessary processing if authentication is invalid
  if (!locals.session) {
    console.warn("[StreamSummarize] Authentication failed: no session (early check)");
    return json(
      {
        error: "Authentication required",
        message: "Session validation failed"
      },
      {
        status: 401,
        headers: {
          "Content-Type": "application/json",
          "Cache-Control": "no-cache",
          "X-Accel-Buffering": "no", // Prevent buffering for SSE endpoints even on error
        },
      }
    );
  }

  // Get backend token early - fail fast if token cannot be obtained
  const cookieHeader = request.headers.get("cookie") || "";
  let token: string | null;
  try {
    token = await getBackendToken(cookieHeader);
    if (!token) {
      console.warn("[StreamSummarize] Authentication failed: no backend token (early check)");
      return json(
        {
          error: "Authentication required",
          message: "Backend token validation failed"
        },
        {
          status: 401,
          headers: {
            "Content-Type": "application/json",
            "Cache-Control": "no-cache",
            "X-Accel-Buffering": "no", // Prevent buffering for SSE endpoints even on error
          },
        }
      );
    }
  } catch (tokenError) {
    console.error("[StreamSummarize] Error getting backend token", {
      error: tokenError instanceof Error ? tokenError.message : String(tokenError),
    });
    return json(
      {
        error: "Authentication failed",
        message: "Failed to obtain backend token"
      },
      {
        status: 401,
        headers: {
          "Content-Type": "application/json",
          "Cache-Control": "no-cache",
          "X-Accel-Buffering": "no", // Prevent buffering for SSE endpoints even on error
        },
      }
    );
  }

  try {
    const body: RequestBody = await request.json();
    console.log("[StreamSummarize] Request body parsed", {
      hasFeedUrl: !!body.feed_url,
      hasArticleId: !!body.article_id,
      hasContent: !!body.content,
      hasTitle: !!body.title,
      contentLength: body.content?.length || 0,
    });

    // Validate that at least feed_url or article_id is provided
    if ((!body.feed_url || typeof body.feed_url !== "string") && (!body.article_id || typeof body.article_id !== "string")) {
      console.warn("[StreamSummarize] Validation failed: missing feed_url or article_id");
      return json(
        { error: "Missing or invalid feed_url or article_id parameter" },
        {
          status: 400,
          headers: {
            "Content-Type": "application/json",
            "Cache-Control": "no-cache",
            "X-Accel-Buffering": "no", // Prevent buffering for SSE endpoints even on error
          },
        },
      );
    }

    // SSRF protection: validate URL before processing (only if feed_url is provided)
    if (body.feed_url) {
      try {
        validateUrlForSSRF(body.feed_url);
        console.log("[StreamSummarize] SSRF validation passed", { feed_url: body.feed_url });
      } catch (error) {
        if (error instanceof Error && error.name === "SSRFValidationError") {
          console.warn("[StreamSummarize] SSRF validation failed", { feed_url: body.feed_url, error: error.message });
          return json(
            { error: "Invalid URL: SSRF protection blocked this request" },
            {
              status: 400,
              headers: {
                "Content-Type": "application/json",
                "Cache-Control": "no-cache",
                "X-Accel-Buffering": "no", // Prevent buffering for SSE endpoints even on error
              },
            },
          );
        }
        throw error;
      }
    }

    // Fetch from alt-backend
    const backendUrl = env.BACKEND_BASE_URL || "http://alt-backend:9000";
    const backendEndpoint = `${backendUrl}/v1/feeds/summarize/stream`;

    // Forward cookies and headers
    const forwardedFor = request.headers.get("x-forwarded-for") || "";
    const forwardedProto = request.headers.get("x-forwarded-proto") || "https";

    console.log("[StreamSummarize] Sending request to backend", {
      backendEndpoint,
      hasToken: !!token,
      payloadSize: JSON.stringify({
        feed_url: body.feed_url,
        article_id: body.article_id,
        content: body.content,
        title: body.title,
      }).length,
    });

    // Set timeout for initial connection (30 seconds)
    // Streaming response itself has no timeout, but connection should establish quickly
    const controller = new AbortController();
    const connectionTimeout = setTimeout(() => {
      console.warn("[StreamSummarize] Connection timeout, aborting request", {
        backendEndpoint,
        elapsed: `${Date.now() - startTime}ms`,
      });
      controller.abort();
    }, 600000); // 10 minute connection timeout for streaming (LLM processing can take time)

    let backendResponse: Response;
    try {
      backendResponse = await fetch(backendEndpoint, {
        method: "POST",
        headers: {
          Cookie: cookieHeader,
          "Content-Type": "application/json",
          "X-Forwarded-For": forwardedFor,
          "X-Forwarded-Proto": forwardedProto,
          "X-Alt-Backend-Token": token,
          // Ensure buffering is disabled for streaming if possible (depends on fetch implementation)
        },
        body: JSON.stringify({
          feed_url: body.feed_url,
          article_id: body.article_id,
          content: body.content,
          title: body.title,
        }),
        cache: "no-store",
        signal: controller.signal,
      });

      // Clear timeout once response is received
      clearTimeout(connectionTimeout);
    } catch (fetchError) {
      clearTimeout(connectionTimeout);
      const fetchErrorTime = Date.now() - startTime;
      console.error("[StreamSummarize] Failed to fetch from backend", {
        error: fetchError instanceof Error ? fetchError.message : String(fetchError),
        stack: fetchError instanceof Error ? fetchError.stack : undefined,
        name: fetchError instanceof Error ? fetchError.name : undefined,
        backendEndpoint,
        responseTime: `${fetchErrorTime}ms`,
      });

      if (fetchError instanceof Error) {
        if (fetchError.name === "AbortError" || fetchError.message.includes("timeout")) {
          console.error("[StreamSummarize] Backend connection timeout", {
            backendEndpoint,
            elapsed: `${fetchErrorTime}ms`,
          });
          return json(
            {
              error: "Backend request timeout",
              message: "The backend service did not respond in time"
            },
            {
              status: 504,
              headers: {
                "Content-Type": "application/json",
                "Cache-Control": "no-cache",
                "X-Accel-Buffering": "no", // Prevent buffering for SSE endpoints even on error
              },
            }
          );
        }
        if (fetchError.message.includes("ECONNREFUSED") || fetchError.message.includes("ENOTFOUND")) {
          console.error("[StreamSummarize] Backend service unavailable", {
            backendEndpoint,
            error: fetchError.message,
          });
          return json(
            {
              error: "Backend service unavailable",
              message: "The backend service is not reachable"
            },
            {
              status: 503,
              headers: {
                "Content-Type": "application/json",
                "Cache-Control": "no-cache",
                "X-Accel-Buffering": "no", // Prevent buffering for SSE endpoints even on error
              },
            }
          );
        }
      }

      console.error("[StreamSummarize] Failed to connect to backend", {
        backendEndpoint,
        error: fetchError instanceof Error ? fetchError.message : String(fetchError),
      });
      return json(
        {
          error: "Failed to connect to backend",
          message: "Unable to establish connection to the backend service"
        },
        {
          status: 502,
          headers: {
            "Content-Type": "application/json",
            "Cache-Control": "no-cache",
            "X-Accel-Buffering": "no", // Prevent buffering for SSE endpoints even on error
          },
        }
      );
    }

    const backendResponseTime = Date.now() - startTime;
    console.log("[StreamSummarize] Backend response received", {
      status: backendResponse.status,
      statusText: backendResponse.statusText,
      contentType: backendResponse.headers.get("Content-Type"),
      hasBody: !!backendResponse.body,
      responseTime: `${backendResponseTime}ms`,
      headers: Object.fromEntries(backendResponse.headers.entries()),
    });

    if (!backendResponse.ok) {
      // Try to read error response body for better error reporting
      let errorBody = "";
      try {
        const errorText = await backendResponse.text();
        errorBody = errorText.substring(0, 500); // Limit error body size
      } catch (e) {
        console.warn("[StreamSummarize] Failed to read error response body", { error: e });
      }

      console.error("[StreamSummarize] Backend API error", {
        status: backendResponse.status,
        statusText: backendResponse.statusText,
        responseTime: `${backendResponseTime}ms`,
        errorBody: errorBody || "No error body",
        headers: Object.fromEntries(backendResponse.headers.entries()),
      });

      // Map backend status codes to appropriate client status codes
      // 401/403 from backend should be passed through, but ensure JSON response
      const clientStatus = backendResponse.status >= 400 && backendResponse.status < 500
        ? backendResponse.status
        : 502; // Gateway error for 5xx from backend

      // For authentication errors (401/403), return JSON error immediately
      // Don't attempt to stream error responses for auth failures
      const isAuthError = backendResponse.status === 401 || backendResponse.status === 403;

      return json(
        {
          error: isAuthError
            ? (backendResponse.status === 403 ? "Forbidden" : "Authentication required")
            : `Backend API error: ${backendResponse.status}`,
          message: errorBody || (isAuthError
            ? "Authentication failed. Please check your session."
            : `Backend returned status ${backendResponse.status}`),
          details: errorBody || undefined,
        },
        {
          status: clientStatus,
          headers: {
            "Content-Type": "application/json",
            "Cache-Control": "no-cache",
            "X-Accel-Buffering": "no", // Prevent buffering even for error responses
          },
        },
      );
    }

    // Check if response body exists
    if (!backendResponse.body) {
      console.error("[StreamSummarize] Backend response body is null", {
        status: backendResponse.status,
        headers: Object.fromEntries(backendResponse.headers.entries()),
      });
      return json(
        { error: "Backend returned empty response" },
        {
          status: 502,
          headers: {
            "Content-Type": "application/json",
            "Cache-Control": "no-cache",
            "X-Accel-Buffering": "no", // Prevent buffering for SSE endpoints even on error
          },
        },
      );
    }

    // Use SSE content type for streaming
    const contentType = "text/event-stream; charset=utf-8";
    console.log("[StreamSummarize] Starting stream forwarding", {
      contentType,
    });

    // Create a ReadableStream to properly forward the backend stream with buffering
    // Buffer chunks and send them every 250ms to reduce request frequency and avoid Cloudflare rate limits
    let bytesForwarded = 0;
    let chunksForwarded = 0;

    // Shared state for cleanup (accessible from both start and cancel)
    let flushInterval: ReturnType<typeof setInterval> | null = null;

    const stream = new ReadableStream({
      async start(controller) {
        const reader = backendResponse.body!.getReader();

        let isStreamDone = false;

        try {
          console.log("[StreamSummarize] Stream reader started (direct streaming)", {
            hasBody: !!backendResponse.body,
            contentType: backendResponse.headers.get("Content-Type"),
          });

          while (true) {
            const { done, value } = await reader.read();
            if (done) {
              isStreamDone = true;
              break;
            }
            if (value) {
              // Direct streaming: Send immediately without buffering
              controller.enqueue(value);
              bytesForwarded += value.length;
              chunksForwarded++;

              // Log occasionally to avoid spam
              if (chunksForwarded <= 3 || chunksForwarded % 50 === 0) {
                // console.log("[StreamSummarize] Chunk forwarded", { chunksForwarded, bytesForwarded });
              }
            }
          }

          const totalTime = Date.now() - startTime;
          console.log("[StreamSummarize] Stream completed", {
            bytesForwarded,
            chunksForwarded,
            totalTime: `${totalTime}ms`,
            avgChunkSize: chunksForwarded > 0 ? Math.round(bytesForwarded / chunksForwarded) : 0,
          });
          controller.close();
        } catch (error) {
          const totalTime = Date.now() - startTime;
          console.error("[StreamSummarize] Error reading from backend stream", {
            error: error instanceof Error ? error.message : String(error),
            stack: error instanceof Error ? error.stack : undefined,
            bytesForwarded,
            chunksForwarded,
            totalTime: `${totalTime}ms`,
          });

          // Send error as JSON before closing the stream
          try {
            const errorMessage = error instanceof Error ? error.message : String(error);
            const errorJson = JSON.stringify({ error: errorMessage, type: "stream_error" });
            // If the stream is still open, try to send the error
            // Note: If we already sent data, this might be appended to the stream content
            // The client needs to handle this gracefully
            controller.enqueue(new TextEncoder().encode("\n" + errorJson));
          } catch (encodeError) {
            console.error("[StreamSummarize] Failed to encode error message", { encodeError });
          }
          controller.close();
        } finally {
          reader.releaseLock();
          console.log("[StreamSummarize] Stream reader released");
        }
      },
      cancel() {
        console.log("[StreamSummarize] Stream cancelled", {
          bytesForwarded,
          chunksForwarded,
          elapsed: `${Date.now() - startTime}ms`,
        });
        // Cleanup if stream is cancelled
        backendResponse.body?.cancel();
      },
    });

    // Return the stream with proper headers for SSE streaming
    return new Response(stream, {
      status: 200,
      headers: {
        "Content-Type": contentType,
        "Cache-Control": "no-cache, no-transform",
        "Connection": "keep-alive",
        "X-Accel-Buffering": "no", // Disable Nginx buffering for SSE
        "X-Content-Type-Options": "nosniff",
        "Access-Control-Allow-Origin": request.headers.get("origin") || "*",
        "Access-Control-Allow-Credentials": "true",
        "Access-Control-Allow-Methods": "POST, OPTIONS",
        "Access-Control-Allow-Headers": "Content-Type, Authorization",
      },
    });

  } catch (error) {
    const totalTime = Date.now() - startTime;
    const errorDetails = {
      error: error instanceof Error ? error.message : String(error),
      stack: error instanceof Error ? error.stack : undefined,
      name: error instanceof Error ? error.name : undefined,
      totalTime: `${totalTime}ms`,
    };

    console.error("[StreamSummarize] Unexpected error in endpoint", errorDetails);

    // Handle specific error types and return JSON errors (never HTML)
    if (error instanceof Error) {
      if (error.name === "AbortError" || error.message.includes("timeout")) {
        return json(
          {
            error: "Request timeout",
            message: "The request timed out while waiting for a response"
          },
          {
            status: 504,
            headers: {
              "Content-Type": "application/json",
              "Cache-Control": "no-cache",
              "X-Accel-Buffering": "no", // Prevent buffering for SSE endpoints even on error
            },
          }
        );
      }
      if (error.message.includes("JSON") || error.name === "SyntaxError") {
        return json(
          {
            error: "Invalid request format",
            message: "The request body is not valid JSON"
          },
          {
            status: 400,
            headers: {
              "Content-Type": "application/json",
              "Cache-Control": "no-cache",
              "X-Accel-Buffering": "no", // Prevent buffering for SSE endpoints even on error
            },
          }
        );
      }
    }

    return json(
      {
        error: "Internal server error",
        message: "An unexpected error occurred while processing the request"
      },
      {
        status: 500,
        headers: {
          "Content-Type": "application/json",
          "Cache-Control": "no-cache",
          "X-Accel-Buffering": "no", // Prevent buffering for SSE endpoints even on error
        },
      }
    );
  }
};

import { json, type RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { getBackendToken } from "$lib/api";
import { validateUrlForSSRF } from "$lib/server/ssrf-validator";

interface RequestBody {
	feed_url: string;
}

interface SummarizeArticleResponse {
	success: boolean;
	summary: string;
	article_id: string;
	feed_url: string;
}

export const POST: RequestHandler = async ({ request, locals }) => {
	try {
		const body: RequestBody = await request.json();

		if (!body.feed_url || typeof body.feed_url !== "string") {
			return json(
				{ error: "Missing or invalid feed_url parameter" },
				{ status: 400 },
			);
		}

		// SSRF protection: validate URL before processing
		try {
			validateUrlForSSRF(body.feed_url);
		} catch (error) {
			if (error instanceof Error && error.name === "SSRFValidationError") {
				return json(
					{ error: "Invalid URL: SSRF protection blocked this request" },
					{ status: 400 },
				);
			}
			throw error;
		}

		// Check authentication
		if (!locals.session) {
			return json({ error: "Authentication required" }, { status: 401 });
		}

		// Get backend token
		const cookieHeader = request.headers.get("cookie") || "";
		const token = await getBackendToken(cookieHeader);

		if (!token) {
			return json({ error: "Authentication required" }, { status: 401 });
		}

		// Fetch from alt-backend
		const backendUrl = env.BACKEND_BASE_URL || "http://alt-backend:9000";
		const backendEndpoint = `${backendUrl}/v1/feeds/summarize/queue`;

		// Forward cookies and headers
		const forwardedFor = request.headers.get("x-forwarded-for") || "";
		const forwardedProto = request.headers.get("x-forwarded-proto") || "https";

		const controller = new AbortController();
		// Short timeout for queue endpoint (should return quickly)
		const timeoutId = setTimeout(() => controller.abort(), 30000); // 30 second timeout

		try {
			const backendResponse = await fetch(backendEndpoint, {
				method: "POST",
				headers: {
					Cookie: cookieHeader,
					"Content-Type": "application/json",
					"X-Forwarded-For": forwardedFor,
					"X-Forwarded-Proto": forwardedProto,
					"X-Alt-Backend-Token": token,
				},
				body: JSON.stringify({ feed_url: body.feed_url }),
				cache: "no-store",
				signal: controller.signal,
			});

			clearTimeout(timeoutId);

			if (!backendResponse.ok) {
				return json(
					{ error: `Backend API error: ${backendResponse.status}` },
					{ status: backendResponse.status },
				);
			}

			const backendData = await backendResponse.json();

			// Check if we got a 202 Accepted (async job) or 200 OK (immediate result)
			if (backendResponse.status === 202) {
				// Async job - return job info for client-side polling
				// Remove /api prefix from status_url if present (callClientAPI adds /sv/api)
				const statusUrl = backendData.status_url || `/v1/feeds/summarize/status/${backendData.job_id}`;
				const normalizedStatusUrl = statusUrl.startsWith("/api/")
					? statusUrl.replace(/^\/api/, "")
					: statusUrl;

				return json(
					{
						success: true,
						job_id: backendData.job_id,
						status_url: normalizedStatusUrl,
						article_id: backendData.article_id,
						feed_url: body.feed_url,
						message: "Summarization job queued",
					},
					{ status: 202 },
				);
			} else {
				// Immediate result (200 OK) - return as-is
				const backendDataTyped: SummarizeArticleResponse = backendData;
				return json(backendDataTyped);
			}
		} catch (fetchError) {
			clearTimeout(timeoutId);
			if (fetchError instanceof Error && fetchError.name === "AbortError") {
				return json({ error: "Request timeout" }, { status: 504 });
			}
			throw fetchError;
		}
	} catch (error) {
		console.error("Error in /api/v1/feeds/summarize:", error);

		if (error instanceof Error && error.name === "AbortError") {
			return json({ error: "Request timeout" }, { status: 504 });
		}

		return json({ error: "Internal server error" }, { status: 500 });
	}
};

import { json, type RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { getBackendToken } from "$lib/api";
import { validateUrlForSSRF } from "$lib/server/ssrf-validator";

interface RequestBody {
	url: string;
}

interface BackendResponse {
	content: string;
	article_id: string;
}

interface SafeResponse {
	content: string; // Will be sanitized
	article_id: string;
}

export const POST: RequestHandler = async ({ request, locals }) => {
	try {
		const body: RequestBody = await request.json();

		if (!body.url || typeof body.url !== "string") {
			return json(
				{ error: "Missing or invalid url parameter" },
				{ status: 400 },
			);
		}

		// SSRF protection: validate URL before processing
		try {
			validateUrlForSSRF(body.url);
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
		const encodedUrl = encodeURIComponent(body.url);
		const backendEndpoint = `${backendUrl}/v1/articles/fetch/content?url=${encodedUrl}`;

		// Forward cookies and headers
		const forwardedFor = request.headers.get("x-forwarded-for") || "";
		const forwardedProto = request.headers.get("x-forwarded-proto") || "https";

		const controller = new AbortController();
		const timeoutId = setTimeout(() => controller.abort(), 15000); // 15 second timeout

		try {
			const backendResponse = await fetch(backendEndpoint, {
				method: "GET", // Backend endpoint only supports GET
				headers: {
					Cookie: cookieHeader,
					"Content-Type": "application/json",
					"X-Forwarded-For": forwardedFor,
					"X-Forwarded-Proto": forwardedProto,
					"X-Alt-Backend-Token": token,
				},
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

			const backendData: BackendResponse = await backendResponse.json();

			// TODO: Sanitize HTML content server-side
			// For now, return as-is (sanitization should be added)
			const safeResponse: SafeResponse = {
				content: backendData.content, // sanitizeForArticle(backendData.content),
				article_id: backendData.article_id,
			};

			return json(safeResponse);
		} catch (fetchError) {
			clearTimeout(timeoutId);
			if (fetchError instanceof Error && fetchError.name === "AbortError") {
				return json({ error: "Request timeout" }, { status: 504 });
			}
			throw fetchError;
		}
	} catch (error) {
		console.error("Error in /api/v1/articles/content:", error);

		if (error instanceof Error && error.name === "AbortError") {
			return json({ error: "Request timeout" }, { status: 504 });
		}

		return json({ error: "Internal server error" }, { status: 500 });
	}
};

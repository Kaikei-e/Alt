import { json, type RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { getBackendToken } from "$lib/api";
import { validateUrlForSSRF } from "$lib/server/ssrf-validator";

interface RequestBody {
	feed_url: string;
	title?: string;
}

interface BackendResponse {
	message: string;
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
		const backendEndpoint = `${backendUrl}/v1/articles/archive`;

		// Forward cookies and headers
		const forwardedFor = request.headers.get("x-forwarded-for") || "";
		const forwardedProto = request.headers.get("x-forwarded-proto") || "https";

		const controller = new AbortController();
		const timeoutId = setTimeout(() => controller.abort(), 30000); // 30 second timeout

		try {
			// Prepare request body for backend
			const backendBody: Record<string, string> = {
				feed_url: body.feed_url,
			};
			if (body.title?.trim()) {
				backendBody.title = body.title.trim();
			}

			const backendResponse = await fetch(backendEndpoint, {
				method: "POST",
				headers: {
					Cookie: cookieHeader,
					"Content-Type": "application/json",
					"X-Forwarded-For": forwardedFor,
					"X-Forwarded-Proto": forwardedProto,
					"X-Alt-Backend-Token": token,
				},
				body: JSON.stringify(backendBody),
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

			return json(backendData);
		} catch (fetchError) {
			clearTimeout(timeoutId);
			if (fetchError instanceof Error && fetchError.name === "AbortError") {
				return json({ error: "Request timeout" }, { status: 504 });
			}
			throw fetchError;
		}
	} catch (error) {
		console.error("Error in /api/v1/articles/archive:", error);

		if (error instanceof Error && error.name === "AbortError") {
			return json({ error: "Request timeout" }, { status: 504 });
		}

		return json({ error: "Internal server error" }, { status: 500 });
	}
};


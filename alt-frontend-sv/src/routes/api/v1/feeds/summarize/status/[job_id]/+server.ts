import { json, type RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { getBackendToken } from "$lib/api";

export const GET: RequestHandler = async ({ params, request, locals }) => {
	try {
		const jobId = params.job_id;

		if (!jobId) {
			return json({ error: "Job ID is required" }, { status: 400 });
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
		const backendEndpoint = `${backendUrl}/v1/feeds/summarize/status/${jobId}`;

		// Forward cookies and headers
		const forwardedFor = request.headers.get("x-forwarded-for") || "";
		const forwardedProto = request.headers.get("x-forwarded-proto") || "https";

		const controller = new AbortController();
		const timeoutId = setTimeout(() => controller.abort(), 10000); // 10 second timeout

		try {
			const backendResponse = await fetch(backendEndpoint, {
				method: "GET",
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
				if (backendResponse.status === 404) {
					return json({ error: "Job not found" }, { status: 404 });
				}
				return json(
					{ error: `Backend API error: ${backendResponse.status}` },
					{ status: backendResponse.status },
				);
			}

			const backendData = await backendResponse.json();

			// Forward the status response as-is
			return json(backendData);
		} catch (fetchError) {
			clearTimeout(timeoutId);
			if (fetchError instanceof Error && fetchError.name === "AbortError") {
				return json({ error: "Request timeout" }, { status: 504 });
			}
			throw fetchError;
		}
	} catch (error) {
		console.error("Error in /api/v1/feeds/summarize/status:", error);

		if (error instanceof Error && error.name === "AbortError") {
			return json({ error: "Request timeout" }, { status: 504 });
		}

		return json({ error: "Internal server error" }, { status: 500 });
	}
};

import { json, type RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { getBackendToken, getCSRFToken } from "$lib/server/auth";

const BACKEND_URL =
	env.BACKEND_CONNECT_URL || "http://alt-butterfly-facade:9250";

export const POST: RequestHandler = async ({ request }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	// V-004: CSRF validation for state-changing operations
	const expectedCSRF = await getCSRFToken(cookieHeader);
	const providedCSRF = request.headers.get("X-CSRF-Token");

	if (!expectedCSRF || expectedCSRF !== providedCSRF) {
		return json({ error: "CSRF validation failed" }, { status: 403 });
	}

	try {
		const token = await getBackendToken(cookieHeader);

		// Read the multipart form data from the client request
		const formData = await request.formData();
		const file = formData.get("file");

		if (!file || !(file instanceof File)) {
			return json({ error: "OPML file is required" }, { status: 400 });
		}

		// Rebuild FormData for the backend request
		const backendFormData = new FormData();
		backendFormData.append("file", file);

		const headers: HeadersInit = {};
		if (token) {
			headers["X-Alt-Backend-Token"] = token;
		}

		const response = await fetch(
			`${BACKEND_URL}/v1/rss-feed-link/import/opml`,
			{
				method: "POST",
				headers,
				body: backendFormData,
			},
		);

		if (!response.ok) {
			const errorText = await response.text().catch(() => "");
			console.error(
				`Backend import failed: ${response.status}`,
				errorText.substring(0, 200),
			);
			return json(
				{ error: `Import failed: ${response.status}` },
				{ status: response.status },
			);
		}

		const result = await response.json();
		return json(result);
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/v1/rss-feed-link/import/opml:", errorMessage);
		return json({ error: errorMessage }, { status: 500 });
	}
};

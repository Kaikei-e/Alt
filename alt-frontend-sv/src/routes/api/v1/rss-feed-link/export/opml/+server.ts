import type { RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { getBackendToken } from "$lib/server/auth";

const BACKEND_BASE_URL = env.BACKEND_BASE_URL || "http://alt-backend:9000";

export const GET: RequestHandler = async ({ request }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	try {
		const token = await getBackendToken(cookieHeader);

		const headers: HeadersInit = {};
		if (token) {
			headers["X-Alt-Backend-Token"] = token;
		}

		const response = await fetch(
			`${BACKEND_BASE_URL}/v1/rss-feed-link/export/opml`,
			{ headers, cache: "no-store" },
		);

		if (!response.ok) {
			const errorText = await response.text().catch(() => "");
			return new Response(
				JSON.stringify({ error: `Backend error: ${response.status}` }),
				{
					status: response.status,
					headers: { "Content-Type": "application/json" },
				},
			);
		}

		const xmlData = await response.arrayBuffer();

		return new Response(xmlData, {
			status: 200,
			headers: {
				"Content-Type": "application/xml",
				"Content-Disposition": 'attachment; filename="alt-feeds.opml"',
			},
		});
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/v1/rss-feed-link/export/opml:", errorMessage);
		return new Response(JSON.stringify({ error: errorMessage }), {
			status: 500,
			headers: { "Content-Type": "application/json" },
		});
	}
};

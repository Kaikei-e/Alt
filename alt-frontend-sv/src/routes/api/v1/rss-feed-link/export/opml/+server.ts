import type { RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { getBackendToken } from "$lib/server/auth";

const BACKEND_URL =
	env.BACKEND_CONNECT_URL || "http://alt-butterfly-facade:9250";

export const GET: RequestHandler = async ({ request }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	try {
		const token = await getBackendToken(cookieHeader);

		const headers: HeadersInit = {};
		if (token) {
			headers["X-Alt-Backend-Token"] = token;
		}

		const response = await fetch(
			`${BACKEND_URL}/v1/rss-feed-link/export/opml`,
			{ headers, cache: "no-store" },
		);

		if (!response.ok) {
			console.error(
				"OPML export backend returned non-OK",
				response.status,
				await response.text().catch(() => ""),
			);
			return new Response(JSON.stringify({ error: "Export failed" }), {
				status: response.status,
				headers: { "Content-Type": "application/json" },
			});
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
		console.error("Error in /api/v1/rss-feed-link/export/opml:", error);
		return new Response(JSON.stringify({ error: "Export failed" }), {
			status: 500,
			headers: { "Content-Type": "application/json" },
		});
	}
};

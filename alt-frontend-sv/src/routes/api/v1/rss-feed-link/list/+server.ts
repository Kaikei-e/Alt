import type { RequestHandler } from "@sveltejs/kit";
import { getFeedLinks } from "#lib/api.js";

export const GET: RequestHandler = async ({ request }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	try {
		const links = await getFeedLinks(cookieHeader);
		return Response.json(links);
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/v1/rss-feed-link/list:", errorMessage);
		return Response.json(
			{ error: "Failed to fetch feed links" },
			{ status: 500 },
		);
	}
};

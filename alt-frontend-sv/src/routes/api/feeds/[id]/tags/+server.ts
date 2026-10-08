import type { RequestHandler } from "@sveltejs/kit";
import { getFeedTagsById } from "#lib/api.js";

export const GET: RequestHandler = async ({ request, params }) => {
	const cookieHeader = request.headers.get("cookie") || "";
	const feedId = params.id;

	if (!feedId) {
		return Response.json({ error: "feed id is required" }, { status: 400 });
	}

	try {
		const response = await getFeedTagsById(cookieHeader, feedId);
		return Response.json(response);
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/feeds/[id]/tags:", {
			message: errorMessage,
			feedId,
			cookiePresent: !!cookieHeader,
		});

		return Response.json(
			{
				error: errorMessage,
				feed_id: feedId,
				tags: [],
			},
			{ status: 500 },
		);
	}
};

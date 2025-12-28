import { json, type RequestHandler } from "@sveltejs/kit";
import { deleteFeedLink } from "$lib/api";

export const DELETE: RequestHandler = async ({ request, params }) => {
	const cookieHeader = request.headers.get("cookie") || "";
	const id = params.id;

	if (!id) {
		return json({ error: "id is required" }, { status: 400 });
	}

	try {
		await deleteFeedLink(cookieHeader, id);
		return json({ success: true });
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/v1/rss-feed-link/[id]:", {
			id,
			error: errorMessage,
		});
		return json({ error: errorMessage }, { status: 500 });
	}
};

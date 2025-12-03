import { json, type RequestHandler } from "@sveltejs/kit";
import { getFeedsWithCursor } from "$lib/api";

export const GET: RequestHandler = async ({ request, url }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	const limit = url.searchParams.get("limit");
	const cursor = url.searchParams.get("cursor");

	try {
		const response = await getFeedsWithCursor(
			cookieHeader,
			cursor || undefined,
			limit ? parseInt(limit, 10) : 20,
		);

		return json(response);
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/v1/feeds/fetch/cursor:", errorMessage);
		return json({ error: errorMessage }, { status: 500 });
	}
};

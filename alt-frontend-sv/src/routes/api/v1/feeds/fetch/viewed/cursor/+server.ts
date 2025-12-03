import { json, type RequestHandler } from "@sveltejs/kit";
import { getReadFeedsWithCursor } from "$lib/api";

export const GET: RequestHandler = async ({ request, url }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	const limit = url.searchParams.get("limit");
	const cursor = url.searchParams.get("cursor");

	try {
		const response = await getReadFeedsWithCursor(
			cookieHeader,
			cursor || undefined,
			limit ? parseInt(limit, 10) : 32,
		);

		return json(response);
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		const errorStack = error instanceof Error ? error.stack : undefined;
		console.error("Error in /api/v1/feeds/fetch/viewed/cursor:", {
			message: errorMessage,
			stack: errorStack,
			cookiePresent: !!cookieHeader,
			limit,
			cursor,
		});
		return json(
			{
				error: errorMessage,
				data: [],
				next_cursor: null,
				has_more: false,
			},
			{ status: 500 },
		);
	}
};

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
		const errorStack = error instanceof Error ? error.stack : undefined;
		const errorName = error instanceof Error ? error.constructor.name : typeof error;

		console.error("Error in /api/v1/feeds/fetch/cursor:", {
			message: errorMessage,
			stack: errorStack,
			errorType: errorName,
			cookiePresent: !!cookieHeader,
			limit,
			cursor: cursor ? cursor.substring(0, 20) + "..." : null,
		});

		// Always return JSON response, never HTML
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

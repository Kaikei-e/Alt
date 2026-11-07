import type { RequestHandler } from "@sveltejs/kit";
import { getFeedsWithCursor } from "#lib/api.js";

const DEFAULT_LIMIT = 20;
const MAX_LIMIT = 100;

export const GET: RequestHandler = async ({ request, url, locals }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	const limitParam = url.searchParams.get("limit");
	const cursor = url.searchParams.get("cursor");

	let limit = DEFAULT_LIMIT;
	if (limitParam !== null && limitParam !== "") {
		const parsed = Number.parseInt(limitParam, 10);
		if (!/^\d+$/.test(limitParam) || Number.isNaN(parsed) || parsed <= 0) {
			return Response.json(
				{ error: "Invalid limit parameter" },
				{ status: 400 },
			);
		}
		limit = Math.min(parsed, MAX_LIMIT);
	}

	try {
		const response = await getFeedsWithCursor(
			cookieHeader,
			cursor || undefined,
			limit,
			locals?.backendToken,
		);

		return Response.json(response);
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		const errorStack = error instanceof Error ? error.stack : undefined;
		const errorName =
			error instanceof Error ? error.constructor.name : typeof error;

		console.error("Error in /api/v1/feeds/fetch/cursor:", {
			message: errorMessage,
			stack: errorStack,
			errorType: errorName,
			cookiePresent: !!cookieHeader,
			limit,
			cursor: cursor ? `${cursor.substring(0, 20)}...` : null,
		});

		// Always return JSON response, never HTML
		return Response.json(
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

import type { RequestHandler } from "@sveltejs/kit";
import { getArticlesByTag } from "#lib/api.js";

const DEFAULT_LIMIT = 20;
const MAX_LIMIT = 100;

export const GET: RequestHandler = async ({ request, url }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	const tagId = url.searchParams.get("tag_id");
	const cursor = url.searchParams.get("cursor");
	const limitStr = url.searchParams.get("limit");

	if (!tagId) {
		return Response.json({ error: "tag_id is required" }, { status: 400 });
	}

	let limit = DEFAULT_LIMIT;
	if (limitStr !== null && limitStr !== "") {
		const parsed = Number.parseInt(limitStr, 10);
		if (!/^\d+$/.test(limitStr) || Number.isNaN(parsed) || parsed <= 0) {
			return Response.json(
				{ error: "Invalid limit parameter" },
				{ status: 400 },
			);
		}
		limit = Math.min(parsed, MAX_LIMIT);
	}

	try {
		const response = await getArticlesByTag(
			cookieHeader,
			tagId,
			cursor || undefined,
			limit,
		);
		return Response.json(response);
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/articles/by-tag:", {
			message: errorMessage,
			tagId,
			cursor,
			limit,
			cookiePresent: !!cookieHeader,
		});

		return Response.json(
			{
				error: "Failed to fetch articles by tag",
				articles: [],
				has_more: false,
			},
			{ status: 500 },
		);
	}
};

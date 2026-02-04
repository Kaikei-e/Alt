import { json, type RequestHandler } from "@sveltejs/kit";
import { getArticlesByTag } from "$lib/api";

export const GET: RequestHandler = async ({ request, url }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	const tagId = url.searchParams.get("tag_id");
	const cursor = url.searchParams.get("cursor");
	const limitStr = url.searchParams.get("limit");
	const limit = limitStr ? Number.parseInt(limitStr, 10) : 20;

	if (!tagId) {
		return json({ error: "tag_id is required" }, { status: 400 });
	}

	try {
		const response = await getArticlesByTag(
			cookieHeader,
			tagId,
			cursor || undefined,
			limit,
		);
		return json(response);
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/articles/by-tag:", {
			message: errorMessage,
			tagId,
			cursor,
			limit,
			cookiePresent: !!cookieHeader,
		});

		return json(
			{
				error: errorMessage,
				articles: [],
				has_more: false,
			},
			{ status: 500 },
		);
	}
};

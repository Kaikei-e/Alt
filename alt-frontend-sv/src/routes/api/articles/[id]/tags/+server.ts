import { json, type RequestHandler } from "@sveltejs/kit";
import { getArticleTags } from "$lib/api";

export const GET: RequestHandler = async ({ request, params }) => {
	const cookieHeader = request.headers.get("cookie") || "";
	const articleId = params.id;

	if (!articleId) {
		return json({ error: "article id is required" }, { status: 400 });
	}

	try {
		const response = await getArticleTags(cookieHeader, articleId);
		return json(response);
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/articles/[id]/tags:", {
			message: errorMessage,
			articleId,
			cookiePresent: !!cookieHeader,
		});

		return json(
			{
				error: errorMessage,
				article_id: articleId,
				tags: [],
			},
			{ status: 500 },
		);
	}
};

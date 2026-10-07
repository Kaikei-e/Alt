import type { RequestHandler } from "@sveltejs/kit";
import { getRandomSubscription } from "#lib/api.js";

export const GET: RequestHandler = async ({ request }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	try {
		const response = await getRandomSubscription(cookieHeader);
		return Response.json(response);
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/feeds/random:", {
			message: errorMessage,
			cookiePresent: !!cookieHeader,
		});

		return Response.json(
			{
				error: "Failed to fetch random feed",
				feed: null,
			},
			{ status: 500 },
		);
	}
};

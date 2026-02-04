import { json, type RequestHandler } from "@sveltejs/kit";
import { getRandomSubscription } from "$lib/api";

export const GET: RequestHandler = async ({ request }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	try {
		const response = await getRandomSubscription(cookieHeader);
		return json(response);
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Error in /api/feeds/random:", {
			message: errorMessage,
			cookiePresent: !!cookieHeader,
		});

		return json(
			{
				error: errorMessage,
				feed: null,
			},
			{ status: 500 },
		);
	}
};

import type { ServerLoad } from "@sveltejs/kit";
import { getFeedsWithCursor } from "$lib/server/feed-api";
import { sanitizeFeed, toRenderFeed } from "$lib/schema/feed";

const INITIAL_FEEDS_LIMIT = 20;

export const load: ServerLoad = async ({ request, locals, fetch }) => {
	const backendToken = locals.backendToken;
	const cookieHeader = request.headers.get("cookie") || "";

	try {
		const response = await getFeedsWithCursor(
			cookieHeader,
			undefined,
			INITIAL_FEEDS_LIMIT,
			backendToken,
			fetch,
		);

		if (
			response.data &&
			Array.isArray(response.data) &&
			response.data.length > 0
		) {
			const initialFeeds = response.data.map((item) => {
				const sanitized = sanitizeFeed(item);
				return toRenderFeed(sanitized, item.tags);
			});

			return { initialFeeds };
		}

		return { initialFeeds: [] };
	} catch (error) {
		console.error("[FeedsPage] Error fetching initial feeds:", error);
		return {
			initialFeeds: [],
			error: "Failed to load feeds",
		};
	}
};

import type { ServerLoad } from "@sveltejs/kit";
import { getFeedsWithCursor } from "$lib/server/feed-api";
import type { BackendFeedItem } from "$lib/schema/feed";
import { sanitizeFeed, toRenderFeed } from "$lib/schema/feed";

const INITIAL_FEEDS_LIMIT = 20;

export const load: ServerLoad = async ({ request, locals }) => {
	const backendToken = locals.backendToken;
	const cookieHeader = request.headers.get("cookie") || "";

	try {
		const response = await getFeedsWithCursor(
			cookieHeader,
			undefined,
			INITIAL_FEEDS_LIMIT,
			backendToken,
		);

		if (
			response.data &&
			Array.isArray(response.data) &&
			response.data.length > 0
		) {
			const backendItems = response.data as BackendFeedItem[];
			const initialFeeds = backendItems.map((item) => {
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

import type { ServerLoad } from "@sveltejs/kit";
import { getFeedsWithCursor } from "$lib/server/feed-api";
import type { BackendFeedItem } from "$lib/schema/feed";
import { sanitizeFeed, toRenderFeed } from "$lib/schema/feed";

const INITIAL_FEEDS_LIMIT = 20;

export const load: ServerLoad = async ({ request, locals }) => {
	const backendToken = locals.backendToken;
	const cookie = request.headers.get("cookie") || "";

	try {
		const response = await getFeedsWithCursor(
			cookie,
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
			const initialFeeds = backendItems.map((item) => ({
				...toRenderFeed(sanitizeFeed(item), item.tags),
				ogImageProxyUrl: item.og_image_proxy_url,
			}));

			return { initialFeeds };
		}

		return { initialFeeds: [] };
	} catch (error) {
		console.error("[visual-preview] Error fetching initial feeds:", error);
		return {
			initialFeeds: [],
			error: "Failed to load feeds",
		};
	}
};

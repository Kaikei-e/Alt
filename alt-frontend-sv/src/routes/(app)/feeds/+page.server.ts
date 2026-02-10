import type { ServerLoad } from "@sveltejs/kit";
import { getFeedsWithCursor } from "$lib/api";
import type { BackendFeedItem } from "$lib/schema/feed";
import { sanitizeFeed, toRenderFeed } from "$lib/schema/feed";

const INITIAL_FEEDS_LIMIT = 20;

/**
 * Fetches initial feeds from the cursor API and converts them to RenderFeed.
 * Used by mobile layout for SSR-delivered initial data.
 */
async function fetchInitialFeeds(
	cookie: string | null,
): Promise<Array<ReturnType<typeof toRenderFeed>>> {
	try {
		const response = await getFeedsWithCursor(
			cookie,
			undefined,
			INITIAL_FEEDS_LIMIT,
		);

		if (
			response.data &&
			Array.isArray(response.data) &&
			response.data.length > 0
		) {
			const backendItems = response.data as BackendFeedItem[];
			return backendItems.map((item) => {
				const sanitized = sanitizeFeed(item);
				return toRenderFeed(sanitized, item.tags);
			});
		}

		return [];
	} catch (error) {
		console.error("[FeedsPage] Error fetching initial feeds:", error);
		return [];
	}
}

export const load: ServerLoad = async ({ request }) => {
	const cookieHeader = request.headers.get("cookie") || "";

	try {
		const initialFeeds = await fetchInitialFeeds(cookieHeader);

		return {
			initialFeeds,
		};
	} catch (error) {
		const errorMessage = error instanceof Error ? error.message : String(error);
		console.error("Failed to load feeds:", {
			message: errorMessage,
			cookieHeader: cookieHeader ? "present" : "missing",
		});

		return {
			initialFeeds: [],
			error: "Failed to load feeds",
		};
	}
};

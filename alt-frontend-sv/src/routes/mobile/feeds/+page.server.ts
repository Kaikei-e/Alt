import type { ServerLoad } from "@sveltejs/kit";
import { getFeedsWithCursor } from "$lib/api";
import type { BackendFeedItem } from "$lib/schema/feed";
import { sanitizeFeed, toRenderFeed } from "$lib/schema/feed";

const INITIAL_FEEDS_LIMIT = 20; // Fetch 20, but render only 3 initially

/**
 * Fetches initial feeds from the cursor API and converts them to RenderFeed
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
			// Type assertion needed because getFeedsWithCursor returns unknown
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
	// Get all cookies as a string
	const cookieHeader = request.headers.get("cookie") || "";

	try {
		// Fetch and convert feeds on the server
		const initialFeeds = await fetchInitialFeeds(cookieHeader);

		return {
			initialFeeds,
		};
	} catch (error) {
		// Log error but don't throw - return empty array instead
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

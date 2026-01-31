import type { RenderFeed } from "$lib/schema/feed";
import type { PageLoad } from "./$types";
import {
	getFeedsWithCursorClient,
	getFeedContentOnTheFlyClient,
} from "$lib/api/client";

// Disable SSR for this page - Connect-RPC client requires browser context
export const ssr = false;

export const load: PageLoad = async () => {
	try {
		const limit = 3;

		// Use Connect-RPC client function
		const feedsData = await getFeedsWithCursorClient(undefined, limit);

		const feeds: RenderFeed[] = feedsData.data;
		const nextCursor = feedsData.next_cursor;

		// Fetch first article content as a non-blocking Promise (streaming pattern)
		// This allows the page to render immediately while article content loads in background
		let articleContentPromise: Promise<string | null> = Promise.resolve(null);
		if (feeds.length > 0) {
			const feedUrl = feeds[0].link;

			articleContentPromise = (async (): Promise<string | null> => {
				try {
					const contentData = await getFeedContentOnTheFlyClient(feedUrl);
					return contentData.content;
				} catch (e) {
					const errorMessage = e instanceof Error ? e.message : String(e);
					console.error("Error fetching initial article content:", {
						error: errorMessage,
						url: feedUrl,
					});
					return null;
				}
			})();
		}

		return {
			initialFeeds: feeds,
			nextCursor,
			articleContentPromise,
		};
	} catch (e) {
		const errorMessage = e instanceof Error ? e.message : String(e);
		const errorStack = e instanceof Error ? e.stack : undefined;
		console.error("Error loading swipe feeds:", {
			message: errorMessage,
			stack: errorStack,
			errorType: e instanceof Error ? e.constructor.name : typeof e,
		});
		return {
			initialFeeds: [],
			nextCursor: null,
			articleContentPromise: Promise.resolve(null),
		};
	}
};

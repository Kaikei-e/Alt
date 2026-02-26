import type { RenderFeed } from "$lib/schema/feed";
import {
	getFeedsWithCursorClient,
	getFeedContentOnTheFlyClient,
} from "$lib/api/client";

// Disable SSR for this page - Connect-RPC client requires browser context
export const ssr = false;

export const load = async () => {
	try {
		const limit = 3;

		// Use Connect-RPC client function
		const feedsData = await getFeedsWithCursorClient(undefined, limit);

		const feeds: RenderFeed[] = feedsData.data;
		const nextCursor = feedsData.next_cursor;

		// Fetch first article content as a non-blocking Promise (streaming pattern)
		// This allows the page to render immediately while article content loads in background
		let articleContentPromise: Promise<{
			content: string | null;
			og_image_url: string | null;
			article_id: string | null;
			feedUrl: string | null;
		}> = Promise.resolve({ content: null, og_image_url: null, article_id: null, feedUrl: null });
		if (feeds.length > 0) {
			const feedUrl = feeds[0].link;

			articleContentPromise = (async () => {
				try {
					const contentData = await getFeedContentOnTheFlyClient(feedUrl);
					return {
						content: contentData.content || null,
						og_image_url: contentData.og_image_url || null,
						article_id: contentData.article_id || null,
						feedUrl,
					};
				} catch (e) {
					const errorMessage = e instanceof Error ? e.message : String(e);
					console.error("Error fetching initial article content:", {
						error: errorMessage,
						url: feedUrl,
					});
					return { content: null, og_image_url: null, article_id: null, feedUrl: null };
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

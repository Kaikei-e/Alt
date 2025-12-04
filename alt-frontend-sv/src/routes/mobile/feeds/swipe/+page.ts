import type { CursorResponse } from "$lib/api";
import { sanitizeFeed, toRenderFeed } from "$lib/schema/feed";
import type { BackendFeedItem } from "$lib/schema/feed";
import type { PageLoad } from "./$types";

export const load: PageLoad = async ({ fetch }) => {
	try {
		const limit = 3;
		const feedsRes = await fetch(`/api/v1/feeds/fetch/cursor?limit=${limit}`);

		if (!feedsRes.ok) {
			throw new Error(`Failed to fetch feeds: ${feedsRes.status}`);
		}

		const feedsData: CursorResponse<BackendFeedItem> = await feedsRes.json();

		let feeds: ReturnType<typeof toRenderFeed>[] = [];
		let nextCursor: string | null = null;

		if (feedsData.data && Array.isArray(feedsData.data)) {
			feeds = feedsData.data.map((item: BackendFeedItem) => {
				const sanitized = sanitizeFeed(item);
				return toRenderFeed(sanitized, item.tags);
			});
			nextCursor = feedsData.next_cursor;
		}

		// Fetch first article content
		let articleContent: string | null = null;
		if (feeds.length > 0) {
			try {
				const contentRes = await fetch(`/api/v1/articles/content`, {
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify({ url: feeds[0].link }),
				});

				if (contentRes.ok) {
					const contentData: { content: string } = await contentRes.json();
					articleContent = contentData.content;
				}
			} catch (e) {
				console.error("Error fetching initial article content:", e);
			}
		}

		return {
			initialFeeds: feeds,
			nextCursor,
			articleContent,
		};
	} catch (e) {
		console.error("Error loading swipe feeds:", e);
		return {
			initialFeeds: [],
			nextCursor: null,
			articleContent: null,
		};
	}
};

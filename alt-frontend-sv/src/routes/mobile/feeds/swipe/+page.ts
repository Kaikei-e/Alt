import type { CursorResponse } from "$lib/api";
import type { BackendFeedItem } from "$lib/schema/feed";
import { sanitizeFeed, toRenderFeed } from "$lib/schema/feed";
import type { PageLoad } from "./$types";

// Base path matches svelte.config.js paths.base
const BASE_PATH = "/sv";

export const load: PageLoad = async ({ fetch }) => {
	try {
		const limit = 3;
		// Include basePath in the API URL for client-side fetch
		const apiUrl = `${BASE_PATH}/api/v1/feeds/fetch/cursor?limit=${limit}`;
		const feedsRes = await fetch(apiUrl);

		if (!feedsRes.ok) {
			const errorText = await feedsRes.text().catch(() => "");
			console.error("Failed to fetch feeds:", {
				status: feedsRes.status,
				statusText: feedsRes.statusText,
				contentType: feedsRes.headers.get("content-type"),
				bodyPreview: errorText.substring(0, 200),
			});
			throw new Error(
				`Failed to fetch feeds: ${feedsRes.status} ${feedsRes.statusText}`,
			);
		}

		// Check Content-Type before parsing JSON
		const contentType = feedsRes.headers.get("content-type") || "";
		const isJson = contentType.includes("application/json");

		if (!isJson) {
			const text = await feedsRes.text().catch(() => "");
			console.error("API returned non-JSON response:", {
				url: apiUrl,
				contentType,
				status: feedsRes.status,
				bodyPreview: text.substring(0, 200),
			});
			throw new Error(
				`API returned non-JSON response (${contentType}). This may indicate a routing error or server-side error page.`,
			);
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
				// Include basePath in the API URL for client-side fetch
				const contentApiUrl = `${BASE_PATH}/api/v1/articles/content`;
				const contentRes = await fetch(contentApiUrl, {
					method: "POST",
					headers: { "Content-Type": "application/json" },
					body: JSON.stringify({ url: feeds[0].link }),
				});

				if (contentRes.ok) {
					// Check Content-Type before parsing JSON
					const contentType = contentRes.headers.get("content-type") || "";
					const isJson = contentType.includes("application/json");

					if (isJson) {
						const contentData: { content: string } = await contentRes.json();
						articleContent = contentData.content;
					} else {
						console.warn("Article content API returned non-JSON response:", {
							contentType,
							status: contentRes.status,
						});
					}
				}
			} catch (e) {
				const errorMessage = e instanceof Error ? e.message : String(e);
				console.error("Error fetching initial article content:", {
					error: errorMessage,
					url: feeds[0].link,
				});
			}
		}

		return {
			initialFeeds: feeds,
			nextCursor,
			articleContent,
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
			articleContent: null,
		};
	}
};

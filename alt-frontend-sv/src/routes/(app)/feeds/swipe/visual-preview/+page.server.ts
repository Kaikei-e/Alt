import type { ServerLoad } from "@sveltejs/kit";
import { getFeedsWithCursor } from "$lib/server/feed-api";
import {
	createServerTransport,
	createServerTransportWithToken,
} from "$lib/connect/transport-server";
import {
	batchPrefetchImages,
	fetchArticleContent,
} from "$lib/connect/articles";
import type { BackendFeedItem } from "$lib/schema/feed";
import { sanitizeFeed, toRenderFeed } from "$lib/schema/feed";

export const load: ServerLoad = async ({ request, locals }) => {
	const backendToken = locals.backendToken;
	const cookie = request.headers.get("cookie") || "";

	try {
		const feedsData = await getFeedsWithCursor(cookie, undefined, 3, backendToken);
		const feeds = (feedsData.data as BackendFeedItem[]).map((item) => ({
			...toRenderFeed(sanitizeFeed(item), item.tags),
			ogImageProxyUrl: item.og_image_proxy_url,
		}));

		// Streamed data: article content is below-the-fold, so defer it
		// to reduce initial SSR HTML size (improves FCP)
		const articleDataPromise =
			feeds.length > 0
				? (async () => {
						try {
							// Reuse cached token to avoid extra auth-hub /session calls
							const transport = backendToken
								? createServerTransportWithToken(backendToken)
								: await createServerTransport(cookie);
							const article = await fetchArticleContent(
								transport,
								feeds[0].link,
							);

							// og_image_proxy_url is no longer in FetchArticleContent response.
							// Use BatchPrefetchImages for proxy URL.
							let firstArticleImageUrl =
								article.ogImageUrl || null;
							if (article.articleId) {
								try {
									const images = await batchPrefetchImages(
										transport,
										[article.articleId],
									);
									if (
										images.length > 0 &&
										images[0].proxyUrl
									) {
										firstArticleImageUrl =
											images[0].proxyUrl;
									}
								} catch {
									// Fall back to raw og_image_url
								}
							}

							return {
								firstArticleImageUrl,
								firstArticleContent: article.content || null,
								firstArticleId: article.articleId || null,
							};
						} catch {
							return {
								firstArticleImageUrl: null,
								firstArticleContent: null,
								firstArticleId: null,
							};
						}
					})()
				: Promise.resolve({
						firstArticleImageUrl: null,
						firstArticleContent: null,
						firstArticleId: null,
					});

		return {
			initialFeeds: feeds,
			nextCursor: feedsData.next_cursor ?? null,
			// Streamed: SvelteKit sends initial HTML immediately, then
			// streams article data when the promise resolves.
			// The OGP image URL is needed for LCP preload, so we await it
			// but the content can be deferred.
			articleData: articleDataPromise,
		};
	} catch (error) {
		console.error("[visual-preview] Server load error:", error);
		return {
			initialFeeds: [],
			nextCursor: null,
			articleData: Promise.resolve({
				firstArticleImageUrl: null,
				firstArticleContent: null,
				firstArticleId: null,
			}),
		};
	}
};

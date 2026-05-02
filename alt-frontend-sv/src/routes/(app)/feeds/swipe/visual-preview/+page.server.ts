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

// Cap the inline article fetch on SSR. The backend now caps the origin fetch
// at 8s; we use 6s on top so the user sees the preview shell quickly even when
// the article cache misses.
const ARTICLE_FETCH_TIMEOUT_MS = 6000;

export const load: ServerLoad = async ({ request, locals }) => {
	const backendToken = locals.backendToken;
	const cookie = request.headers.get("cookie") || "";

	const emptyArticleData = {
		firstArticleImageUrl: null,
		firstArticleContent: null,
		firstArticleId: null,
	};

	try {
		const feedsData = await getFeedsWithCursor(
			cookie,
			undefined,
			3,
			backendToken,
		);
		const feeds = (feedsData.data as BackendFeedItem[]).map((item) => ({
			...toRenderFeed(sanitizeFeed(item), item.tags),
			ogImageProxyUrl: item.og_image_proxy_url,
		}));

		// Streamed data: article content is below-the-fold, so defer it.
		// SvelteKit sends initial HTML immediately and resolves this promise on the client.
		const articleDataPromise: Promise<{
			firstArticleImageUrl: string | null;
			firstArticleContent: string | null;
			firstArticleId: string | null;
		}> =
			feeds.length > 0
				? loadFirstArticle(feeds[0], cookie, backendToken)
				: Promise.resolve(emptyArticleData);

		return {
			initialFeeds: feeds,
			nextCursor: feedsData.next_cursor ?? null,
			articleData: articleDataPromise,
		};
	} catch (error) {
		console.error("[visual-preview] Server load error:", error);
		return {
			initialFeeds: [],
			nextCursor: null,
			articleData: Promise.resolve(emptyArticleData),
		};
	}
};

async function loadFirstArticle(
	firstFeed: { link: string; ogImageProxyUrl?: string | null },
	cookie: string,
	backendToken: string | null,
): Promise<{
	firstArticleImageUrl: string | null;
	firstArticleContent: string | null;
	firstArticleId: string | null;
}> {
	const abort = new AbortController();
	const timer = setTimeout(() => abort.abort(), ARTICLE_FETCH_TIMEOUT_MS);

	// Pre-set the LCP image to the feed-level proxy URL. This gives the client
	// an OGP image to preload even if fetchArticleContent / batchPrefetchImages
	// time out.
	let firstArticleImageUrl = firstFeed.ogImageProxyUrl ?? null;

	try {
		const transport = backendToken
			? createServerTransportWithToken(backendToken)
			: await createServerTransport(cookie);

		const article = await fetchArticleContent(
			transport,
			firstFeed.link,
			abort.signal,
		);

		// Prefer the proxy URL from the article, then the feed-level proxy URL,
		// finally the raw og:image. batchPrefetchImages is only consulted when
		// neither is available — and we run it in parallel with a short timeout
		// of its own so it never blocks SSR past ARTICLE_FETCH_TIMEOUT_MS.
		if (!firstArticleImageUrl && article.ogImageProxyUrl) {
			firstArticleImageUrl = article.ogImageProxyUrl;
		} else if (!firstArticleImageUrl && article.articleId) {
			try {
				const images = await batchPrefetchImages(transport, [
					article.articleId,
				]);
				if (images.length > 0 && images[0].proxyUrl) {
					firstArticleImageUrl = images[0].proxyUrl;
				}
			} catch {
				// Fall back to raw og_image_url
			}
		}

		if (!firstArticleImageUrl) {
			firstArticleImageUrl = article.ogImageUrl || null;
		}

		return {
			firstArticleImageUrl,
			firstArticleContent: article.content || null,
			firstArticleId: article.articleId || null,
		};
	} catch {
		return {
			firstArticleImageUrl,
			firstArticleContent: null,
			firstArticleId: null,
		};
	} finally {
		clearTimeout(timer);
	}
}

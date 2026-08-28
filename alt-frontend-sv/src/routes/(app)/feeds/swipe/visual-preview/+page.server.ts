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
import { sanitizeFeed, toRenderFeed } from "$lib/schema/feed";

// Cap the inline article fetch on SSR. The backend now caps the origin fetch
// at 8s; we use 6s on top so the user sees the preview shell quickly even when
// the article cache misses.
const ARTICLE_FETCH_TIMEOUT_MS = 6000;

export const load: ServerLoad = async ({ request, locals, fetch }) => {
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
			fetch,
		);
		const feeds = feedsData.data.map((item) => ({
			...toRenderFeed(sanitizeFeed(item), item.tags),
			ogImageProxyUrl: item.og_image_proxy_url,
		}));

		// Resolve the first-article data BEFORE returning so the SSR response is
		// a single, fully-formed HTML payload. SvelteKit's deferred-promise
		// streaming triggers chunked-transfer tails that iOS Safari treats as an
		// incomplete document until the trailing 0\r\n\r\n arrives — under Bun
		// 1.3.x that terminator is racy enough to surface "Cannot Open the Page"
		// on slow upstream tails. The 6s AbortController inside loadFirstArticle
		// still caps the wait; we just no longer stream the resolution.
		const articleData =
			feeds.length > 0
				? await loadFirstArticle(feeds[0]!, cookie, backendToken, fetch)
				: emptyArticleData;

		return {
			initialFeeds: feeds,
			nextCursor: feedsData.next_cursor ?? null,
			articleData,
		};
	} catch (error) {
		console.error("[visual-preview] Server load error:", error);
		return {
			initialFeeds: [],
			nextCursor: null,
			articleData: emptyArticleData,
		};
	}
};

async function loadFirstArticle(
	firstFeed: { link: string; ogImageProxyUrl?: string | null },
	cookie: string,
	backendToken: string | null,
	fetchFn?: typeof fetch,
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
	//
	// Every value this variable may take is a signed `/v1/images/proxy/...`
	// path. It is emitted as `<link rel="preload" as="image">` and seeded into
	// the client's og-image cache, from where it becomes the first card's
	// `<img src>` — so a publisher's own URL here is an unproxied cross-origin
	// request in the reader's browser, outside the rate limiting, SSRF
	// validation, domain allow-list and re-encoding the image proxy applies.
	// `|| null`, not `?? null`: an absent og_image_proxy_url arrives from the
	// wire as "", and an empty string is not a URL — it is "no image", and
	// saying so as null keeps every branch below on one falsy value.
	let firstArticleImageUrl = firstFeed.ogImageProxyUrl || null;

	try {
		const transport = backendToken
			? createServerTransportWithToken(backendToken, fetchFn)
			: await createServerTransport(cookie, fetchFn);

		const article = await fetchArticleContent(
			transport,
			firstFeed.link,
			abort.signal,
		);

		// Feed-level proxy URL first, then the article's own, then
		// batchPrefetchImages — three sources of the same signed path, tried in
		// the order of what costs least. `article.ogImageUrl` is deliberately
		// NOT a fourth: it is the publisher's URL, and it used to be taken
		// precisely in the common case where the feed row had no
		// `og_image_proxy_url` and the batch came back empty. When none of the
		// three yields a signed path, the card shows no preview — which is the
		// honest answer, and the only one that keeps the reader's browser off a
		// third-party host.
		if (!firstArticleImageUrl && article.ogImageProxyUrl) {
			firstArticleImageUrl = article.ogImageProxyUrl;
		} else if (!firstArticleImageUrl && article.articleId) {
			try {
				const images = await batchPrefetchImages(transport, [
					article.articleId,
				]);
				if (images.length > 0 && images[0]!.proxyUrl) {
					firstArticleImageUrl = images[0]!.proxyUrl;
				}
			} catch {
				// No signed URL from this source either; the card goes without.
			}
		}

		return {
			firstArticleImageUrl: firstArticleImageUrl || null,
			firstArticleContent: article.content || null,
			firstArticleId: article.articleId || null,
		};
	} catch {
		return {
			firstArticleImageUrl: firstArticleImageUrl || null,
			firstArticleContent: null,
			firstArticleId: null,
		};
	} finally {
		clearTimeout(timer);
	}
}

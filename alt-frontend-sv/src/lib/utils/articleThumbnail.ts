/**
 * Obtains an article's thumbnail at the moment a reader is looking at that
 * article, rather than from anything held in advance.
 *
 * Two places can answer. The stored og:image is free — it costs the publisher
 * nothing and is what the feed surfaces already show. It is also short-lived:
 * `article_heads` is purged on a retention window, so an item resurfaced by
 * Knowledge Home weeks later has nothing stored, and asking the origin is the
 * only way left. That request is made once per article per session and its
 * refusal is remembered exactly like its answer, so a reader reopening the same
 * pane does not ask a publisher twice.
 */

const UUID_PATTERN =
	/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

const ORIGIN_FETCH_TIMEOUT_MS = 15_000;

export interface ArticleThumbnailSource {
	/** The image already held for this article, if any. Costs no origin request. */
	fetchCachedProxyUrl: (articleId: string) => Promise<string | null>;
	/** The article's canonical source URL, for callers that do not carry one. */
	fetchSourceUrl: (articleId: string) => Promise<string | null>;
	/** Reads the page's og:image now. This is the request a publisher sees. */
	fetchOriginProxyUrl: (sourceUrl: string) => Promise<string | null>;
}

export interface ArticleThumbnailResolver {
	/**
	 * Resolves this article's proxy URL, or null if it has none to give.
	 * Pass `sourceUrl` when the caller already holds it — it saves a lookup.
	 */
	resolve: (
		articleId: string,
		sourceUrl?: string | null,
	) => Promise<string | null>;
}

export function createArticleThumbnailResolver(
	source: ArticleThumbnailSource,
): ArticleThumbnailResolver {
	// Articles already asked about, with the answer. A null value is "asked,
	// and there was nothing" — kept so reopening the pane does not ask again.
	const settled = new Map<string, string | null>();
	const inFlight = new Map<string, Promise<string | null>>();

	async function lookup(
		articleId: string,
		sourceUrl: string | null | undefined,
	): Promise<string | null> {
		const cached = await source.fetchCachedProxyUrl(articleId);
		if (cached) return cached;

		const url = sourceUrl || (await source.fetchSourceUrl(articleId));
		if (!url) return null;

		return await source.fetchOriginProxyUrl(url);
	}

	function resolve(
		articleId: string,
		sourceUrl?: string | null,
	): Promise<string | null> {
		// An id the backend would reject as a scope is not one to spend a
		// lookup on.
		if (!UUID_PATTERN.test(articleId)) return Promise.resolve(null);

		if (settled.has(articleId)) {
			return Promise.resolve(settled.get(articleId) ?? null);
		}
		const pending = inFlight.get(articleId);
		if (pending) return pending;

		const promise = lookup(articleId, sourceUrl)
			.then((url) => {
				settled.set(articleId, url);
				return url;
			})
			.catch(() => {
				// A transport failure is ours, not the publisher's answer.
				// Leaving it unsettled lets a later mount try again rather than
				// turning one blip into a blank card for the whole session.
				return null;
			})
			.finally(() => {
				inFlight.delete(articleId);
			});

		inFlight.set(articleId, promise);
		return promise;
	}

	return { resolve };
}

/**
 * The resolver the Augur surfaces share.
 *
 * One instance per session on purpose: the "already asked" memory is what keeps
 * the ask pane and the conversation thread — which render the same article
 * moments apart — from resolving it twice.
 */
let shared: ArticleThumbnailResolver | null = null;

export function articleThumbnailResolver(): ArticleThumbnailResolver {
	if (!shared) {
		shared = createArticleThumbnailResolver({
			fetchCachedProxyUrl: async (articleId) => {
				const { createClientTransport } = await import(
					"$lib/connect/transport-client"
				);
				const { batchPrefetchImages } = await import("$lib/connect/articles");
				const images = await batchPrefetchImages(createClientTransport(), [
					articleId,
				]);
				return images.find((i) => i.articleId === articleId)?.proxyUrl || null;
			},
			fetchSourceUrl: async (articleId) => {
				const { createClientTransport } = await import(
					"$lib/connect/transport-client"
				);
				const { getArticleSourceURL } = await import("$lib/connect/articles");
				const source = await getArticleSourceURL(
					createClientTransport(),
					articleId,
				);
				return source.url || null;
			},
			fetchOriginProxyUrl: async (sourceUrl) => {
				const { createClientTransport } = await import(
					"$lib/connect/transport-client"
				);
				const { fetchArticleContent } = await import("$lib/connect/articles");
				const article = await fetchArticleContent(
					createClientTransport(),
					sourceUrl,
					// FetchArticleContent is allowed two minutes because a reader
					// waiting for the article body will wait. A reader waiting for a
					// thumbnail will not, so the card gives up early and settles on
					// its icon. The work still lands on the server, so the next open
					// takes the cheap stored path.
					AbortSignal.timeout(ORIGIN_FETCH_TIMEOUT_MS),
				);
				return article.ogImageProxyUrl || null;
			},
		});
	}
	return shared;
}

/**
 * SSR loader for the mobile swipe surface.
 *
 * `firstArticleImageUrl` is the LCP image: it is emitted into the SSR HTML as
 * `<link rel="preload" as="image">` and seeded into the client's og-image cache,
 * from where it becomes the first card's `<img src>`. So whatever this function
 * returns is rendered — and the only URL that may be rendered is a signed
 * `/v1/images/proxy/...` path.
 *
 * The loader used to end with `firstArticleImageUrl = article.ogImageUrl`, the
 * publisher's own URL, as a last resort. That fallback fired exactly when the
 * feed row carried no `og_image_proxy_url` and `batchPrefetchImages` returned
 * nothing — the common case — and put a third-party host into the preload hint
 * and the card, outside every gate the image proxy exists to apply.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";

const { getFeedsWithCursor, fetchArticleContent, batchPrefetchImages } =
	vi.hoisted(() => ({
		getFeedsWithCursor: vi.fn(),
		fetchArticleContent: vi.fn(),
		batchPrefetchImages: vi.fn(),
	}));

vi.mock("$lib/server/feed-api", () => ({ getFeedsWithCursor }));
vi.mock("$lib/connect/articles", () => ({
	fetchArticleContent,
	batchPrefetchImages,
}));
vi.mock("$lib/connect/transport-server", () => ({
	createServerTransport: vi.fn(async () => ({})),
	createServerTransportWithToken: vi.fn(() => ({})),
}));

import { load } from "./+page.server";

const RAW_OG_IMAGE = "https://cdn.example.com/hero.jpg";
const FEED_PROXY_URL = "/v1/images/proxy/testsig/ZmVlZC1sZXZlbA";
const ARTICLE_PROXY_URL = "/v1/images/proxy/testsig/YXJ0aWNsZS1sZXZlbA";
const BATCH_PROXY_URL = "/v1/images/proxy/testsig/YmF0Y2gtbGV2ZWw";

function feedRow(overrides: Record<string, unknown> = {}) {
	return {
		title: "AI Trends",
		description: "Deep dive into the ecosystem.",
		link: "https://example.com/ai-trends",
		published: "2025-01-15T10:00:00Z",
		created_at: "2025-01-15T09:00:00Z",
		article_id: "article-1",
		feed_id: "feeds-row-uuid-1",
		og_image_proxy_url: "",
		...overrides,
	};
}

/** What this loader returns, as far as these tests care. */
interface LoadResult {
	articleData: {
		firstArticleImageUrl: string | null;
		firstArticleContent: string | null;
		firstArticleId: string | null;
	};
}

/**
 * The SvelteKit `load` event, cut down to the three fields this loader reads.
 * Cast through `unknown` rather than restating SvelteKit's own event type.
 */
const loadEvent = () =>
	({
		request: new Request("http://localhost/feeds/swipe/visual-preview"),
		locals: { backendToken: "token" },
		fetch: globalThis.fetch,
	}) as unknown as Parameters<typeof load>[0];

const runLoad = async (): Promise<LoadResult> =>
	(await load(loadEvent())) as LoadResult;

describe("visual-preview SSR load — the LCP image", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		getFeedsWithCursor.mockResolvedValue({
			data: [feedRow()],
			next_cursor: null,
		});
		batchPrefetchImages.mockResolvedValue([]);
		fetchArticleContent.mockResolvedValue({
			url: "https://example.com/ai-trends",
			content: "<p>Body.</p>",
			articleId: "article-1",
			ogImageUrl: RAW_OG_IMAGE,
			ogImageProxyUrl: "",
		});
	});

	it("never falls back to the publisher's own URL", async () => {
		// Nothing produced a signed URL: the feed row has none, the article has
		// none, and batchPrefetchImages came back empty. The card shows no
		// preview, which is the honest answer — the alternative is an unproxied
		// cross-origin request the reader never asked for.
		const result = await runLoad();

		expect(result.articleData.firstArticleImageUrl).toBeNull();
	});

	it("prefers the feed row's proxy URL", async () => {
		getFeedsWithCursor.mockResolvedValue({
			data: [feedRow({ og_image_proxy_url: FEED_PROXY_URL })],
			next_cursor: null,
		});

		const result = await runLoad();

		expect(result.articleData.firstArticleImageUrl).toBe(FEED_PROXY_URL);
	});

	it("takes the article's proxy URL when the feed row has none", async () => {
		fetchArticleContent.mockResolvedValue({
			url: "https://example.com/ai-trends",
			content: "<p>Body.</p>",
			articleId: "article-1",
			ogImageUrl: RAW_OG_IMAGE,
			ogImageProxyUrl: ARTICLE_PROXY_URL,
		});

		const result = await runLoad();

		expect(result.articleData.firstArticleImageUrl).toBe(ARTICLE_PROXY_URL);
	});

	it("falls back to batchPrefetchImages, which also only yields proxy URLs", async () => {
		batchPrefetchImages.mockResolvedValue([
			{ articleId: "article-1", proxyUrl: BATCH_PROXY_URL, isCached: false },
		]);

		const result = await runLoad();

		expect(result.articleData.firstArticleImageUrl).toBe(BATCH_PROXY_URL);
	});

	it("returns whatever it returns as a proxy path, never a cross-origin URL", async () => {
		// The invariant stated once, over every branch above: this value is
		// rendered as an <img src> and preloaded as an image, so it is either a
		// same-origin proxy path or nothing at all.
		for (const setup of [
			() => {},
			() =>
				getFeedsWithCursor.mockResolvedValue({
					data: [feedRow({ og_image_proxy_url: FEED_PROXY_URL })],
					next_cursor: null,
				}),
			() =>
				fetchArticleContent.mockResolvedValue({
					url: "https://example.com/ai-trends",
					content: "",
					articleId: "article-1",
					ogImageUrl: RAW_OG_IMAGE,
					ogImageProxyUrl: ARTICLE_PROXY_URL,
				}),
			() =>
				batchPrefetchImages.mockResolvedValue([
					{ articleId: "article-1", proxyUrl: BATCH_PROXY_URL },
				]),
		]) {
			setup();
			const url = (await runLoad()).articleData.firstArticleImageUrl;
			if (url !== null) expect(url).toMatch(/^\/v1\/images\/proxy\//);
			expect(url).not.toBe(RAW_OG_IMAGE);
		}
	});

	it("keeps the article body and id, which are unrelated to the image", async () => {
		const result = await runLoad();

		expect(result.articleData.firstArticleContent).toBe("<p>Body.</p>");
		expect(result.articleData.firstArticleId).toBe("article-1");
	});
});

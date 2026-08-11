import { beforeEach, describe, expect, it, vi } from "vitest";
import {
	type ArticleThumbnailSource,
	createArticleThumbnailResolver,
} from "./articleThumbnail";

const ARTICLE_ID = "c6463b44-589f-4959-879f-40eda019f95a";
const SOURCE_URL = "https://www.theguardian.com/film/article";
const PROXY_URL = "/api/og-image?u=https%3A%2F%2Fcdn.example%2Fog.png";

function createSource(
	overrides: Partial<ArticleThumbnailSource> = {},
): ArticleThumbnailSource {
	return {
		fetchCachedProxyUrl: vi.fn(async () => null),
		fetchSourceUrl: vi.fn(async () => SOURCE_URL),
		fetchOriginProxyUrl: vi.fn(async () => PROXY_URL),
		...overrides,
	};
}

describe("createArticleThumbnailResolver", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("serves the stored image without touching the publisher", async () => {
		const source = createSource({
			fetchCachedProxyUrl: vi.fn(async () => PROXY_URL),
		});
		const resolver = createArticleThumbnailResolver(source);

		await expect(resolver.resolve(ARTICLE_ID)).resolves.toBe(PROXY_URL);
		expect(source.fetchOriginProxyUrl).not.toHaveBeenCalled();
		expect(source.fetchSourceUrl).not.toHaveBeenCalled();
	});

	// article_heads is purged after its retention window, so a Knowledge Home
	// item older than that has no stored image and the origin is the only place
	// left to ask.
	it("falls back to the origin when nothing is stored", async () => {
		const source = createSource();
		const resolver = createArticleThumbnailResolver(source);

		await expect(resolver.resolve(ARTICLE_ID, SOURCE_URL)).resolves.toBe(
			PROXY_URL,
		);
		expect(source.fetchOriginProxyUrl).toHaveBeenCalledWith(SOURCE_URL);
		expect(source.fetchSourceUrl).not.toHaveBeenCalled();
	});

	it("looks the source url up when the caller does not carry one", async () => {
		const source = createSource();
		const resolver = createArticleThumbnailResolver(source);

		await expect(resolver.resolve(ARTICLE_ID)).resolves.toBe(PROXY_URL);
		expect(source.fetchSourceUrl).toHaveBeenCalledWith(ARTICLE_ID);
		expect(source.fetchOriginProxyUrl).toHaveBeenCalledWith(SOURCE_URL);
	});

	it("gives up without a fetch when no source url can be found", async () => {
		const source = createSource({ fetchSourceUrl: vi.fn(async () => null) });
		const resolver = createArticleThumbnailResolver(source);

		await expect(resolver.resolve(ARTICLE_ID)).resolves.toBeNull();
		expect(source.fetchOriginProxyUrl).not.toHaveBeenCalled();
	});

	// One reader opening the same pane twice must not cost the publisher two
	// requests — the refusal is remembered exactly like the resolution is.
	it("asks once per article, answer or refusal", async () => {
		const source = createSource({
			fetchOriginProxyUrl: vi.fn(async () => null),
		});
		const resolver = createArticleThumbnailResolver(source);

		await resolver.resolve(ARTICLE_ID, SOURCE_URL);
		await expect(resolver.resolve(ARTICLE_ID, SOURCE_URL)).resolves.toBeNull();

		expect(source.fetchCachedProxyUrl).toHaveBeenCalledTimes(1);
		expect(source.fetchOriginProxyUrl).toHaveBeenCalledTimes(1);
	});

	it("shares one in-flight resolution between concurrent callers", async () => {
		const source = createSource();
		const resolver = createArticleThumbnailResolver(source);

		const [a, b] = await Promise.all([
			resolver.resolve(ARTICLE_ID, SOURCE_URL),
			resolver.resolve(ARTICLE_ID, SOURCE_URL),
		]);

		expect([a, b]).toEqual([PROXY_URL, PROXY_URL]);
		expect(source.fetchCachedProxyUrl).toHaveBeenCalledTimes(1);
	});

	// A transport failure is ours, not the publisher's answer. Remembering it
	// would turn one blip into a permanently blank card for the session.
	it("does not remember a failure of our own", async () => {
		const fetchCachedProxyUrl = vi
			.fn<ArticleThumbnailSource["fetchCachedProxyUrl"]>()
			.mockRejectedValueOnce(new Error("network down"))
			.mockResolvedValueOnce(PROXY_URL);
		const resolver = createArticleThumbnailResolver(
			createSource({ fetchCachedProxyUrl }),
		);

		await expect(resolver.resolve(ARTICLE_ID)).resolves.toBeNull();
		await expect(resolver.resolve(ARTICLE_ID)).resolves.toBe(PROXY_URL);
		expect(fetchCachedProxyUrl).toHaveBeenCalledTimes(2);
	});

	it("refuses an id the backend would reject as a scope", async () => {
		const source = createSource();
		const resolver = createArticleThumbnailResolver(source);

		await expect(resolver.resolve("abc123")).resolves.toBeNull();
		await expect(resolver.resolve("")).resolves.toBeNull();
		expect(source.fetchCachedProxyUrl).not.toHaveBeenCalled();
	});
});

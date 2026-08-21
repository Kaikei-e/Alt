import { Code, ConnectError } from "@connectrpc/connect";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Mock the client API to prevent transitive $app/* resolution
vi.mock("$lib/api/client", () => ({
	getFeedContentOnTheFlyClient: vi.fn(),
}));

import { getFeedContentOnTheFlyClient } from "$lib/api/client";
import type { FeedContentOnTheFlyResponse } from "$lib/api/client/articles";
import type { RenderFeed } from "$lib/schema/feed";
import { ArticlePrefetcher } from "./articlePrefetcher";

const mockedGetContent = vi.mocked(getFeedContentOnTheFlyClient);

function makeFeed(id: string, url: string): RenderFeed {
	return {
		id,
		normalizedUrl: url,
		link: url,
		title: "Test",
		description: "",
		published: "",
		author: "",
		feedSource: "",
	} as unknown as RenderFeed;
}

describe("ArticlePrefetcher", () => {
	let prefetcher: ArticlePrefetcher;

	beforeEach(() => {
		// resetAllMocks (not clearAllMocks) so mockResolvedValueOnce queues
		// from a previous test do not leak into the next.
		vi.resetAllMocks();
		vi.useFakeTimers();
		prefetcher = new ArticlePrefetcher();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	describe("cache eviction", () => {
		it("does not evict active card's OG image when prefetching ahead", async () => {
			// Seed the active card's cache
			prefetcher.seedCache(
				"https://example.com/active",
				"<p>Active</p>",
				"art-active",
				null,
				"https://proxy/active.jpg",
			);

			// Verify active card has OG image
			expect(prefetcher.getCachedOgImage("https://example.com/active")).toBe(
				"https://proxy/active.jpg",
			);

			// Seed 30 more entries to trigger eviction
			for (let i = 1; i <= 30; i++) {
				prefetcher.seedCache(
					`https://example.com/feed-${i}`,
					`<p>Feed ${i}</p>`,
					`art-${i}`,
					null,
					`https://proxy/feed-${i}.jpg`,
				);
			}

			// With MAX_CACHE_SIZE=30, the active card should still be in cache
			// (it was the 1st of 31 entries, so it gets evicted only after 30 new entries)
			// The key point: with the old MAX_CACHE_SIZE=10, this would have been evicted
			// after just 10 new entries. With 30, we have enough headroom for prefetchAhead=10.

			// Simulate the realistic scenario: active + 10 prefetched
			const prefetcher2 = new ArticlePrefetcher();
			prefetcher2.seedCache(
				"https://example.com/current",
				"<p>Current</p>",
				"art-current",
				null,
				"https://proxy/current.jpg",
			);

			for (let i = 1; i <= 10; i++) {
				prefetcher2.seedCache(
					`https://example.com/next-${i}`,
					`<p>Next ${i}</p>`,
					`art-next-${i}`,
					null,
					`https://proxy/next-${i}.jpg`,
				);
			}

			// Active card's OG image must survive with 11 entries (well under MAX_CACHE_SIZE=30)
			expect(prefetcher2.getCachedOgImage("https://example.com/current")).toBe(
				"https://proxy/current.jpg",
			);
			expect(prefetcher2.getCachedContent("https://example.com/current")).toBe(
				"<p>Current</p>",
			);
		});
	});

	describe("per-host serialization", () => {
		it("serializes prefetches to the same host (no parallel fan-out)", async () => {
			let resolveFirst: (value: FeedContentOnTheFlyResponse) => void = () => {};
			const firstCallPromise = new Promise<FeedContentOnTheFlyResponse>(
				(resolve) => {
					resolveFirst = resolve;
				},
			);

			mockedGetContent.mockImplementationOnce(() => firstCallPromise);
			mockedGetContent.mockResolvedValueOnce({
				content: "<p>Second</p>",
				article_id: "art-2",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://zenn.dev/article-1"),
				makeFeed("2", "https://zenn.dev/article-2"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 2);

			// Advance past both PREFETCH_DELAY windows.
			await vi.advanceTimersByTimeAsync(1100);

			// Only the first call should have started — the second is queued
			// behind the first promise on the same host.
			expect(mockedGetContent).toHaveBeenCalledTimes(1);
			expect(mockedGetContent).toHaveBeenNthCalledWith(
				1,
				"https://zenn.dev/article-1",
			);

			// Resolve first → second runs.
			resolveFirst({
				content: "<p>First</p>",
				article_id: "art-1",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			await vi.waitFor(() => {
				expect(prefetcher.getCachedContent("https://zenn.dev/article-2")).toBe(
					"<p>Second</p>",
				);
			});

			expect(mockedGetContent).toHaveBeenCalledTimes(2);
			expect(mockedGetContent).toHaveBeenNthCalledWith(
				2,
				"https://zenn.dev/article-2",
			);
		});

		it("allows prefetch for different hosts concurrently", async () => {
			mockedGetContent
				.mockResolvedValueOnce({
					content: "<p>Zenn</p>",
					article_id: "art-z",
					og_image_url: null,
				} as unknown as FeedContentOnTheFlyResponse)
				.mockResolvedValueOnce({
					content: "<p>Example</p>",
					article_id: "art-e",
					og_image_url: null,
				} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://active.com/page"),
				makeFeed("1", "https://zenn.dev/article-1"),
				makeFeed("2", "https://example.com/article-2"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 2);

			// Use async timer advancement to flush microtasks between timer steps
			await vi.advanceTimersByTimeAsync(1100);

			// Wait for both fetches to fully complete (cache populated)
			await vi.waitFor(() => {
				expect(prefetcher.getCachedContent("https://zenn.dev/article-1")).toBe(
					"<p>Zenn</p>",
				);
				expect(
					prefetcher.getCachedContent("https://example.com/article-2"),
				).toBe("<p>Example</p>");
			});

			// Both calls should have been made (different hosts)
			expect(mockedGetContent).toHaveBeenCalledTimes(2);
		});
	});

	describe("429 cooldown", () => {
		it("applies a 30s cooldown to a host after ConnectError ResourceExhausted", async () => {
			// First call rejects with 429; subsequent prefetches on the same host
			// must skip until the cooldown lifts.
			mockedGetContent.mockRejectedValueOnce(
				new ConnectError("rate limited", Code.ResourceExhausted),
			);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-1"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);

			await vi.advanceTimersByTimeAsync(1_100);
			await vi.waitFor(() => {
				expect(mockedGetContent).toHaveBeenCalledTimes(1);
			});

			// Re-trigger before cooldown expires — must NOT call client again.
			await vi.advanceTimersByTimeAsync(15_000);
			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(2_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			// Past 30s — cooldown lifted, prefetch resumes.
			mockedGetContent.mockResolvedValueOnce({
				content: "<p>OK</p>",
				article_id: "art-ok",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			await vi.advanceTimersByTimeAsync(20_000);
			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(1_100);

			await vi.waitFor(() => {
				expect(mockedGetContent).toHaveBeenCalledTimes(2);
			});
		});

		it("cooldown is per-host: a 429 on host A does not block host B", async () => {
			mockedGetContent.mockRejectedValueOnce(
				new ConnectError("rate limited", Code.ResourceExhausted),
			);
			mockedGetContent.mockResolvedValueOnce({
				content: "<p>B</p>",
				article_id: "art-b",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://active.com/page"),
				makeFeed("1", "https://dev.to/article-a"),
				makeFeed("2", "https://zenn.dev/article-b"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 2);
			await vi.advanceTimersByTimeAsync(1_100);

			await vi.waitFor(() => {
				expect(prefetcher.getCachedContent("https://zenn.dev/article-b")).toBe(
					"<p>B</p>",
				);
			});

			expect(mockedGetContent).toHaveBeenCalledTimes(2);
		});

		it("honors Retry-After header (delta-seconds) when shorter than the default 30s", async () => {
			const meta = new Headers({ "Retry-After": "2" });
			mockedGetContent.mockRejectedValueOnce(
				new ConnectError("rate limited", Code.ResourceExhausted, meta),
			);
			mockedGetContent.mockResolvedValueOnce({
				content: "<p>OK</p>",
				article_id: "art-ok",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-1"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(1_100);

			// Wait through Retry-After window and re-trigger.
			await vi.advanceTimersByTimeAsync(2_500);
			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(1_100);

			await vi.waitFor(() => {
				expect(prefetcher.getCachedContent("https://dev.to/article-1")).toBe(
					"<p>OK</p>",
				);
			});
		});

		it("seedCache bypasses cooldown (manual path is unaffected)", async () => {
			mockedGetContent.mockRejectedValueOnce(
				new ConnectError("rate limited", Code.ResourceExhausted),
			);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-1"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(1_100);
			await vi.waitFor(() => {
				expect(mockedGetContent).toHaveBeenCalledTimes(1);
			});

			// Manual seed during cooldown still populates the cache.
			prefetcher.seedCache(
				"https://dev.to/article-1",
				"<p>Manual</p>",
				"art-m",
				null,
			);

			expect(prefetcher.getCachedContent("https://dev.to/article-1")).toBe(
				"<p>Manual</p>",
			);
		});
	});

	// A breaker rejection from the BFF means nothing the client asks for can be
	// served, whatever the host. The per-host 429 cooldown would have let every
	// other host through and burned the whole lookahead ladder against the open
	// breaker.
	//
	// It has to say so, though. CodeUnavailable on its own does not mean this —
	// alt-backend returns the same code for a single publisher that never
	// answered — so the pause is armed by the declared scope, not by the code.
	describe("global pause on Unavailable (circuit breaker / 503)", () => {
		const breakerError = () =>
			new ConnectError(
				"Service temporarily unavailable due to circuit breaker",
				Code.Unavailable,
				new Headers({ "X-Alt-Failure-Scope": "global" }),
			);

		it("pauses prefetch on every host, not just the one that returned 503", async () => {
			mockedGetContent.mockRejectedValueOnce(breakerError());

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-a"),
				makeFeed("2", "https://zenn.dev/article-b"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 2);
			await vi.advanceTimersByTimeAsync(1_100);

			// zenn.dev is a different host: a per-host cooldown would have let it
			// through on the second rung of the ladder.
			await vi.advanceTimersByTimeAsync(5_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);
			expect(mockedGetContent).toHaveBeenCalledWith("https://dev.to/article-a");
		});

		it("holds a queued article for the whole window, then fires it", async () => {
			mockedGetContent.mockRejectedValueOnce(breakerError());
			mockedGetContent.mockResolvedValue({
				content: "<p>Later</p>",
				article_id: "art-later",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-a"),
				makeFeed("2", "https://zenn.dev/article-b"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 2);
			await vi.advanceTimersByTimeAsync(1_100);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			await vi.advanceTimersByTimeAsync(15_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			// No further triggerPrefetch(): the queued article must come back on
			// its own rather than latch into the card's error state until the
			// reader happens to swipe again.
			await vi.advanceTimersByTimeAsync(20_000);
			await vi.waitFor(() => {
				expect(prefetcher.getCachedContent("https://zenn.dev/article-b")).toBe(
					"<p>Later</p>",
				);
			});
		});

		it("honors a Retry-After shorter than the default pause", async () => {
			const meta = new Headers({
				"Retry-After": "2",
				"X-Alt-Failure-Scope": "global",
			});
			mockedGetContent.mockRejectedValueOnce(
				new ConnectError("breaker open", Code.Unavailable, meta),
			);
			mockedGetContent.mockResolvedValue({
				content: "<p>Back</p>",
				article_id: "art-back",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-a"),
				makeFeed("2", "https://zenn.dev/article-b"),
			];
			prefetcher.triggerPrefetch(feeds, 0, 2);
			await vi.advanceTimersByTimeAsync(1_100);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			// Well inside the 30s default, but past the 2s the gateway asked for.
			await vi.advanceTimersByTimeAsync(3_000);
			await vi.waitFor(() => {
				expect(prefetcher.getCachedContent("https://zenn.dev/article-b")).toBe(
					"<p>Back</p>",
				);
			});
		});
	});

	// The other half of Unavailable, and by far the commoner one: a publisher
	// that did not answer. alt-backend maps that to CodeUnavailable too, so
	// arming the all-hosts pause on the code alone let a single dead link black
	// the reader out for a full cooldown — every card after it fell straight to
	// "Source content unavailable" without a request ever being sent.
	describe("host-scoped Unavailable (one publisher, not the gateway)", () => {
		const publisherError = (headers?: Record<string, string>) =>
			new ConnectError(
				"the source site did not respond; please try again later",
				Code.Unavailable,
				new Headers({ "X-Alt-Failure-Scope": "host", ...headers }),
			);

		it("keeps prefetching other hosts when only the publisher failed", async () => {
			mockedGetContent.mockRejectedValueOnce(publisherError());
			mockedGetContent.mockResolvedValue({
				content: "<p>Other host</p>",
				article_id: "art-b",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-a"),
				makeFeed("2", "https://zenn.dev/article-b"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 2);
			await vi.advanceTimersByTimeAsync(1_100);

			await vi.waitFor(() => {
				expect(prefetcher.getCachedContent("https://zenn.dev/article-b")).toBe(
					"<p>Other host</p>",
				);
			});
			expect(mockedGetContent).toHaveBeenCalledWith(
				"https://zenn.dev/article-b",
			);
		});

		it("still holds off the host that failed", async () => {
			mockedGetContent.mockRejectedValue(publisherError());

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-a"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			// A second article on the same host must wait out that host's
			// cooldown rather than walk into the same refusal.
			const more = [...feeds, makeFeed("2", "https://dev.to/article-b")];
			prefetcher.triggerPrefetch(more, 1, 1);
			await vi.advanceTimersByTimeAsync(1_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);
		});

		it("serves a different host's card immediately", async () => {
			mockedGetContent.mockRejectedValueOnce(publisherError());
			mockedGetContent.mockResolvedValue({
				content: "<p>Elsewhere</p>",
				article_id: "art-elsewhere",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-a"),
			];
			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			await expect(
				prefetcher.ensureContent("https://unrelated.example/article"),
			).resolves.toBe("<p>Elsewhere</p>");
		});

		it("treats an Unavailable that declares no scope as host-scoped", async () => {
			// Nothing downstream can tell an unstamped CodeUnavailable apart from
			// a per-article one, so the ambiguous case takes the cheaper mistake:
			// pausing one host wrongly costs one card, pausing every host wrongly
			// costs the session — and the BFF's breaker still bounds the load.
			mockedGetContent.mockRejectedValueOnce(
				new ConnectError("external site returned 503", Code.Unavailable),
			);
			mockedGetContent.mockResolvedValue({
				content: "<p>Elsewhere</p>",
				article_id: "art-elsewhere",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-a"),
			];
			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			await expect(
				prefetcher.ensureContent("https://unrelated.example/article"),
			).resolves.toBe("<p>Elsewhere</p>");
		});

		it("honors a Retry-After on the host cooldown", async () => {
			mockedGetContent.mockRejectedValueOnce(
				publisherError({ "Retry-After": "2" }),
			);
			mockedGetContent.mockResolvedValue({
				content: "<p>Back</p>",
				article_id: "art-back",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-a"),
			];
			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			// Well inside the 30s default, but past the 2s the server asked for.
			await vi.advanceTimersByTimeAsync(2_500);
			await expect(
				prefetcher.ensureContent("https://dev.to/article-a"),
			).resolves.toBe("<p>Back</p>");
		});
	});

	// A cooldown exists to stop the lookahead ladder from walking into a gate it
	// has already been refused at. The card the reader is looking at is not the
	// ladder — it is one request they explicitly asked for, and answering it
	// without ever sending anything is what turned a few seconds of backoff into
	// a card that read as permanently broken, retry button included.
	describe("foreground probe across an active cooldown", () => {
		const breakerError = () =>
			new ConnectError(
				"Service temporarily unavailable due to circuit breaker",
				Code.Unavailable,
				new Headers({ "X-Alt-Failure-Scope": "global" }),
			);

		async function armGlobalPause() {
			mockedGetContent.mockRejectedValueOnce(breakerError());
			prefetcher.triggerPrefetch(
				[
					makeFeed("0", "https://example.com/active"),
					makeFeed("1", "https://dev.to/article-a"),
				],
				0,
				1,
			);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);
		}

		it("sends the visible card's request even while every host is paused", async () => {
			await armGlobalPause();
			mockedGetContent.mockResolvedValue({
				content: "<p>Probed</p>",
				article_id: "art-probe",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			await expect(
				prefetcher.ensureContent("https://unrelated.example/article"),
			).resolves.toBe("<p>Probed</p>");
			expect(mockedGetContent).toHaveBeenCalledTimes(2);
		});

		it("sends it while only that article's host is cooling down", async () => {
			mockedGetContent.mockRejectedValueOnce(
				new ConnectError(
					"the source site did not respond",
					Code.Unavailable,
					new Headers({ "X-Alt-Failure-Scope": "host" }),
				),
			);
			prefetcher.triggerPrefetch(
				[
					makeFeed("0", "https://example.com/active"),
					makeFeed("1", "https://dev.to/article-a"),
				],
				0,
				1,
			);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			mockedGetContent.mockResolvedValue({
				content: "<p>Probed</p>",
				article_id: "art-probe",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			// Same host that just refused: the reader swiped onto it and tapped.
			await expect(
				prefetcher.ensureContent("https://dev.to/article-b"),
			).resolves.toBe("<p>Probed</p>");
		});

		it("spaces repeated attempts so a held retry button cannot hammer", async () => {
			await armGlobalPause();
			mockedGetContent.mockRejectedValue(breakerError());

			await expect(
				prefetcher.ensureContent("https://a.example/article"),
			).resolves.toBeNull();
			expect(mockedGetContent).toHaveBeenCalledTimes(2);

			// Straight after the probe: refused without a request, which is what
			// the cooldown is for once the reader has had their attempt.
			await expect(
				prefetcher.ensureContent("https://b.example/article"),
			).resolves.toBeNull();
			expect(mockedGetContent).toHaveBeenCalledTimes(2);

			await vi.advanceTimersByTimeAsync(5_500);
			await expect(
				prefetcher.ensureContent("https://c.example/article"),
			).resolves.toBeNull();
			expect(mockedGetContent).toHaveBeenCalledTimes(3);
		});

		it("leaves the background ladder suppressed while the foreground probes", async () => {
			await armGlobalPause();
			mockedGetContent.mockResolvedValue({
				content: "<p>Probed</p>",
				article_id: "art-probe",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			// The probe is the reader's request, not a licence for the ladder:
			// only the one call goes out, from ensureContent.
			await prefetcher.ensureContent("https://unrelated.example/article");
			expect(mockedGetContent).toHaveBeenCalledTimes(2);
			expect(mockedGetContent).toHaveBeenLastCalledWith(
				"https://unrelated.example/article",
			);
		});

		it("lifts the pause when a probe gets through", async () => {
			await armGlobalPause();
			mockedGetContent.mockResolvedValue({
				content: "<p>Probed</p>",
				article_id: "art-probe",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			await prefetcher.ensureContent("https://unrelated.example/article");

			// A request just completed, so whatever the pause was guarding is
			// over. Sitting out the rest of the window would keep the ladder
			// suppressed and the next card blank for no reason.
			await expect(
				prefetcher.ensureContent("https://another.example/article"),
			).resolves.toBe("<p>Probed</p>");
			expect(mockedGetContent).toHaveBeenCalledTimes(3);
		});
	});

	describe("onArticleIdCached callback", () => {
		it("fires when prefetchContent resolves with an article_id", async () => {
			const callback = vi.fn();
			prefetcher.setOnArticleIdCached(callback);

			mockedGetContent.mockResolvedValueOnce({
				content: "<p>Hello</p>",
				article_id: "art-123",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://example.com/article"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);

			// Advance past PREFETCH_DELAY (500ms)
			vi.advanceTimersByTime(600);

			// Wait for the async fetch to complete
			await vi.waitFor(() => {
				expect(callback).toHaveBeenCalledWith(
					"https://example.com/article",
					"art-123",
				);
			});
		});

		it("does not fire when article_id is absent", async () => {
			const callback = vi.fn();
			prefetcher.setOnArticleIdCached(callback);

			mockedGetContent.mockResolvedValueOnce({
				content: "<p>No article ID</p>",
				article_id: "",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://example.com/no-id"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);
			vi.advanceTimersByTime(600);

			// Wait for fetch to complete
			await vi.waitFor(() => {
				expect(mockedGetContent).toHaveBeenCalled();
			});

			// Callback should NOT have been called
			expect(callback).not.toHaveBeenCalled();
		});

		it("can be cleared by passing null", async () => {
			const callback = vi.fn();
			prefetcher.setOnArticleIdCached(callback);
			prefetcher.setOnArticleIdCached(null);

			mockedGetContent.mockResolvedValueOnce({
				content: "<p>test</p>",
				article_id: "art-456",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://example.com/cleared"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);
			vi.advanceTimersByTime(600);

			await vi.waitFor(() => {
				expect(mockedGetContent).toHaveBeenCalled();
			});

			expect(callback).not.toHaveBeenCalled();
		});
	});

	// Regression: swiping a few cards stopped fetching article bodies for the
	// rest of the session. SwipeFeedScreen's batch-image path seeds the cache
	// with `getCachedContent(url) || ""` for feeds it has never fetched, which
	// wrote an empty body. `contentCache.has()` then reported a hit and
	// suppressed every future prefetch, while getCachedContent() kept reporting
	// a miss — so the card fell back to "Source content unavailable" forever.
	describe("empty-body cache poisoning", () => {
		it("does not treat an image-only seed as cached content", () => {
			const url = "https://example.com/image-only";

			// This is exactly what triggerBatchImagePrefetch passes for a feed
			// whose body has never been fetched.
			prefetcher.seedCache(url, "", "art-1", null, "https://proxy/1.jpg");

			expect(prefetcher.getCachedContent(url)).toBeNull();
			// The image metadata is the point of that seed and must survive.
			expect(prefetcher.getCachedOgImage(url)).toBe("https://proxy/1.jpg");
		});

		it("still prefetches a feed whose image metadata was seeded first", async () => {
			const url = "https://example.com/image-only";
			prefetcher.seedCache(url, "", "art-1", null, "https://proxy/1.jpg");

			mockedGetContent.mockResolvedValueOnce({
				content: "<p>Body</p>",
				article_id: "art-1",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", url),
			];
			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(600);

			await vi.waitFor(() => {
				expect(prefetcher.getCachedContent(url)).toBe("<p>Body</p>");
			});
		});

		it("never overwrites a real body with an empty one", () => {
			const url = "https://example.com/article";
			prefetcher.seedCache(url, "<p>Real</p>", "art-1", null);
			prefetcher.seedCache(url, "", "art-1", null, "https://proxy/1.jpg");

			expect(prefetcher.getCachedContent(url)).toBe("<p>Real</p>");
		});
	});

	describe("ensureContent (visible-card fetch)", () => {
		it("joins an in-flight prefetch instead of issuing a duplicate request", async () => {
			let resolveFetch: (value: FeedContentOnTheFlyResponse) => void = () => {};
			mockedGetContent.mockImplementationOnce(
				() =>
					new Promise<FeedContentOnTheFlyResponse>((resolve) => {
						resolveFetch = resolve;
					}),
			);

			const url = "https://zenn.dev/article-1";
			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", url),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			// The card mounts while the prefetch is still in flight. Before the
			// fix it saw getCachedContent() === null (the "loading" sentinel
			// reads as a miss) and fired its own request into the host limiter.
			const fromCard = prefetcher.ensureContent(url);

			resolveFetch({
				content: "<p>Body</p>",
				article_id: "art-1",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			await expect(fromCard).resolves.toBe("<p>Body</p>");
			expect(mockedGetContent).toHaveBeenCalledTimes(1);
		});

		it("returns cached content without touching the network", async () => {
			const url = "https://example.com/cached";
			prefetcher.seedCache(url, "<p>Cached</p>", "art-1", null);

			await expect(prefetcher.ensureContent(url)).resolves.toBe(
				"<p>Cached</p>",
			);
			expect(mockedGetContent).not.toHaveBeenCalled();
		});

		it("rations rather than refuses the visible card during a host cooldown", async () => {
			mockedGetContent.mockRejectedValue(
				new ConnectError("rate limited", Code.ResourceExhausted),
			);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-1"),
			];
			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			await vi.waitFor(() => {
				expect(mockedGetContent).toHaveBeenCalledTimes(1);
			});

			// The reader swiped onto this host and asked for the body: one
			// attempt goes out, even though the ladder is held off.
			await expect(
				prefetcher.ensureContent("https://dev.to/article-2"),
			).resolves.toBeNull();
			expect(mockedGetContent).toHaveBeenCalledTimes(2);

			// Asking again straight away is the case the cooldown is for.
			await expect(
				prefetcher.ensureContent("https://dev.to/article-3"),
			).resolves.toBeNull();
			expect(mockedGetContent).toHaveBeenCalledTimes(2);
		});

		it("fetches when nothing is cached or in flight", async () => {
			mockedGetContent.mockResolvedValueOnce({
				content: "<p>Fresh</p>",
				article_id: "art-1",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			await expect(
				prefetcher.ensureContent("https://example.com/fresh"),
			).resolves.toBe("<p>Fresh</p>");
			expect(mockedGetContent).toHaveBeenCalledTimes(1);
		});
	});

	describe("triggerPrefetch scheduling", () => {
		it("does not starve a scheduled prefetch when re-triggered inside the delay window", async () => {
			mockedGetContent.mockResolvedValue({
				content: "<p>Body</p>",
				article_id: "art-1",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://example.com/next-1"),
			];

			// SwipeFeedScreen's prefetch $effect re-runs on every `feeds`
			// reassignment (handleArticleIdResolved rebuilds the array), which
			// happens more often than PREFETCH_DELAY. Clearing and re-arming the
			// timer each time meant it never fired.
			for (let i = 0; i < 5; i++) {
				prefetcher.triggerPrefetch(feeds, 0, 1);
				await vi.advanceTimersByTimeAsync(400);
			}

			await vi.waitFor(() => {
				expect(mockedGetContent).toHaveBeenCalledWith(
					"https://example.com/next-1",
				);
			});
		});

		it("cancels a scheduled prefetch once its feed leaves the lookahead window", async () => {
			mockedGetContent.mockResolvedValue({
				content: "<p>Body</p>",
				article_id: "art-1",
				og_image_url: null,
			} as unknown as FeedContentOnTheFlyResponse);

			const feeds = [
				makeFeed("0", "https://example.com/a"),
				makeFeed("1", "https://example.com/b"),
				makeFeed("2", "https://example.com/c"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1); // schedules /b
			await vi.advanceTimersByTimeAsync(100);
			prefetcher.triggerPrefetch(feeds, 1, 1); // window moves to /c
			await vi.advanceTimersByTimeAsync(1_000);

			await vi.waitFor(() => {
				expect(mockedGetContent).toHaveBeenCalledWith("https://example.com/c");
			});
			expect(mockedGetContent).not.toHaveBeenCalledWith(
				"https://example.com/b",
			);
		});
	});

	// A prefetch that actually failed used to be forgotten. runFetch deleted the
	// cache entry, set a cooldown, warned and returned null — nothing re-armed
	// it — so the card behind that rung stayed blank until the reader happened
	// to swipe past it again. The ledger gives those items a bounded, jittered
	// second chance, and never lets one through a gate that is still shut.
	describe("retry ledger for a prefetch that actually failed", () => {
		afterEach(() => {
			// The jitter spies below are per-test; the module mock is a vi.fn()
			// from the factory and is untouched by restoreAllMocks.
			vi.restoreAllMocks();
		});

		/** Alt's own host-slot gate: 429, deliberately unstamped (ADR-000963 §2). */
		const slotGate429 = (headers?: Record<string, string>) =>
			new ConnectError(
				"wait for host slot: context deadline exceeded",
				Code.ResourceExhausted,
				new Headers({ ...headers }),
			);

		/** A publisher that did not answer: Unavailable stamped to its host. */
		const publisherDown = (headers?: Record<string, string>) =>
			new ConnectError(
				"the source site did not respond",
				Code.Unavailable,
				new Headers({ "X-Alt-Failure-Scope": "host", ...headers }),
			);

		const body = (content: string) =>
			({
				content,
				article_id: "art-retry",
				og_image_url: null,
			}) as unknown as FeedContentOnTheFlyResponse;

		const twoFeeds = () => [
			makeFeed("0", "https://example.com/active"),
			makeFeed("1", "https://dev.to/article-1"),
		];

		it("brings a retryable failure back without another swipe", async () => {
			vi.spyOn(Math, "random").mockReturnValue(0);
			mockedGetContent.mockRejectedValueOnce(slotGate429());
			mockedGetContent.mockResolvedValue(body("<p>Second try</p>"));

			prefetcher.triggerPrefetch(twoFeeds(), 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);
			expect(
				prefetcher.getCachedContent("https://dev.to/article-1"),
			).toBeNull();

			// No further triggerPrefetch(): the ledger has to bring it back on
			// its own, exactly as the global-pause re-arm already does.
			await vi.advanceTimersByTimeAsync(35_000);
			await vi.waitFor(() => {
				expect(prefetcher.getCachedContent("https://dev.to/article-1")).toBe(
					"<p>Second try</p>",
				);
			});
			expect(mockedGetContent).toHaveBeenCalledTimes(2);
		});

		it("spends at most two retries on one item", async () => {
			// Google SRE's per-request budget is three attempts; the original try
			// is one of them. Retries multiply browser -> BFF -> backend ->
			// publisher, so the client's share of that budget is two.
			vi.spyOn(Math, "random").mockReturnValue(0);
			mockedGetContent.mockRejectedValue(slotGate429());

			prefetcher.triggerPrefetch(twoFeeds(), 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			await vi.advanceTimersByTimeAsync(30_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(2);

			await vi.advanceTimersByTimeAsync(30_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(3);

			// Budget spent. However long the card stays in the window, the
			// ladder stops asking.
			await vi.advanceTimersByTimeAsync(300_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(3);
		});

		it("never retries a failure that will fail the same way again", async () => {
			mockedGetContent.mockRejectedValue(
				new ConnectError("no extractable body", Code.NotFound),
			);

			prefetcher.triggerPrefetch(twoFeeds(), 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			await vi.advanceTimersByTimeAsync(300_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);
		});

		it("never retries a 429 the publisher itself issued", async () => {
			// ADR-000884's incident, restated: re-sending into a host that just
			// said "too many requests" is the storm the cooldown exists to stop.
			vi.spyOn(Math, "random").mockReturnValue(0);
			mockedGetContent.mockRejectedValue(
				new ConnectError(
					"external site returned 429",
					Code.ResourceExhausted,
					new Headers({ "X-Alt-Failure-Scope": "host" }),
				),
			);

			prefetcher.triggerPrefetch(twoFeeds(), 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			await vi.advanceTimersByTimeAsync(300_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);
		});

		it("waits out the host cooldown before retrying", async () => {
			vi.spyOn(Math, "random").mockReturnValue(0);
			mockedGetContent.mockRejectedValueOnce(
				slotGate429({ "Retry-After": "10" }),
			);
			mockedGetContent.mockResolvedValue(body("<p>Later</p>"));

			prefetcher.triggerPrefetch(twoFeeds(), 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			// Inside the window the server asked for: nothing goes out.
			await vi.advanceTimersByTimeAsync(9_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			await vi.advanceTimersByTimeAsync(2_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(2);
		});

		it("clamps an absurd Retry-After so one bad header cannot wedge the reader", async () => {
			vi.spyOn(Math, "random").mockReturnValue(0);
			// A day. Honoured literally this blacks the host out for the rest of
			// the session — and the cooldown it feeds is client-side only, so
			// nothing about the backend's own politeness budget is relaxed.
			mockedGetContent.mockRejectedValueOnce(
				slotGate429({ "Retry-After": "86400" }),
			);
			mockedGetContent.mockResolvedValue(body("<p>Unwedged</p>"));

			prefetcher.triggerPrefetch(twoFeeds(), 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			await vi.advanceTimersByTimeAsync(58_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			await vi.advanceTimersByTimeAsync(4_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(2);
		});

		it("spreads retries with full jitter instead of a fixed backoff", async () => {
			// AWS Brooker 2015: delay = random(0, min(cap, base * 2**attempt)).
			// The randomness is the point. Deterministic backoff puts every
			// client that failed in the same incident back on the wire at the
			// same instant, so the recovery knocks the host over again.
			async function measureRetryDelay(roll: number): Promise<number> {
				vi.resetAllMocks();
				const randomSpy = vi.spyOn(Math, "random").mockReturnValue(roll);
				const isolated = new ArticlePrefetcher();

				mockedGetContent.mockRejectedValueOnce(
					slotGate429({ "Retry-After": "2" }),
				);
				let firedAt = 0;
				mockedGetContent.mockImplementation(() => {
					firedAt = Date.now();
					return Promise.reject(new ConnectError("still down", Code.NotFound));
				});

				isolated.triggerPrefetch(twoFeeds(), 0, 1);
				await vi.advanceTimersByTimeAsync(600);
				const armedAt = Date.now();
				await vi.advanceTimersByTimeAsync(20_000);
				randomSpy.mockRestore();

				expect(firedAt).toBeGreaterThan(0);
				return firedAt - armedAt;
			}

			const atFloor = await measureRetryDelay(0);
			const atCeiling = await measureRetryDelay(1);

			// Both land no earlier than the 2s the server asked for...
			expect(atFloor).toBeGreaterThanOrEqual(1_000);
			// ...and the top of the jitter window is strictly later than the
			// bottom, which a fixed backoff could never produce.
			expect(atCeiling).toBeGreaterThan(atFloor);
		});

		it("drops the ledger entry when the item leaves the lookahead window", async () => {
			vi.spyOn(Math, "random").mockReturnValue(0);
			mockedGetContent.mockRejectedValueOnce(slotGate429());
			mockedGetContent.mockResolvedValue(body("<p>Whatever</p>"));

			const feeds = [
				makeFeed("0", "https://example.com/active"),
				makeFeed("1", "https://dev.to/article-1"),
				makeFeed("2", "https://zenn.dev/article-2"),
			];

			prefetcher.triggerPrefetch(feeds, 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);
			expect(mockedGetContent).toHaveBeenCalledWith("https://dev.to/article-1");

			// The reader swiped on. article-1 is behind them now; a retry would
			// be spending a publisher's budget on a card nobody is heading for.
			prefetcher.triggerPrefetch(feeds, 1, 1);
			await vi.advanceTimersByTimeAsync(300_000);

			const article1Calls = mockedGetContent.mock.calls.filter(
				(call) => call[0] === "https://dev.to/article-1",
			);
			expect(article1Calls).toHaveLength(1);
		});

		it("re-arms rather than firing a retry into a cooldown that appeared meanwhile", async () => {
			vi.spyOn(Math, "random").mockReturnValue(0);
			// Booked for ~2.5s by the server's own Retry-After.
			mockedGetContent.mockRejectedValueOnce(
				publisherDown({ "Retry-After": "2" }),
			);

			prefetcher.triggerPrefetch(twoFeeds(), 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			// Before it fires, the reader's own card hits the same host and is
			// refused again — this time with no Retry-After, so the default 30s
			// shuts the gate the retry was about to walk into.
			mockedGetContent.mockRejectedValueOnce(publisherDown());
			await prefetcher.ensureContent("https://dev.to/other-article");
			expect(mockedGetContent).toHaveBeenCalledTimes(2);

			mockedGetContent.mockResolvedValue(body("<p>Eventually</p>"));

			// Past the original booking, still inside the fresh cooldown.
			await vi.advanceTimersByTimeAsync(3_000);
			expect(mockedGetContent).toHaveBeenCalledTimes(2);

			// And once the gate opens it does come back.
			await vi.advanceTimersByTimeAsync(40_000);
			await vi.waitFor(() => {
				expect(prefetcher.getCachedContent("https://dev.to/article-1")).toBe(
					"<p>Eventually</p>",
				);
			});
		});

		it("books one retry per item however many paths fail it", async () => {
			vi.spyOn(Math, "random").mockReturnValue(0);
			mockedGetContent.mockRejectedValue(publisherDown());

			prefetcher.triggerPrefetch(twoFeeds(), 0, 1);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			// The card for that same article mounts while it is still in the
			// lookahead window and asks for the body itself. Its failure is the
			// reader's, not the ladder's: it must not stack a second timer nor
			// spend an attempt from the ladder's budget.
			await prefetcher.ensureContent("https://dev.to/article-1");
			expect(mockedGetContent).toHaveBeenCalledTimes(2);

			await vi.advanceTimersByTimeAsync(300_000);
			// 2 original attempts + exactly 2 ledger retries.
			expect(mockedGetContent).toHaveBeenCalledTimes(4);
		});
	});

	// triggerPrefetch encoded priority as PREFETCH_DELAY * i and nothing else,
	// so a rung whose host was already cooling down or already busy still held
	// the earliest slot — and the article behind it, on a host that could have
	// answered immediately, waited out a delay for a request that was never
	// going to be sent. Ordering is free: the same articles, the same number of
	// requests, the same ladder pacing, just handed out to the hosts that can
	// use them. prefetchAhead stays where ADR-000884 put it.
	describe("host-aware lookahead ordering", () => {
		const body = (content: string) =>
			({
				content,
				article_id: "art-order",
				og_image_url: null,
			}) as unknown as FeedContentOnTheFlyResponse;

		it("gives the first rung to a host that is not cooling down", async () => {
			// Host-stamped so the ledger stays out of it: this is about order.
			mockedGetContent.mockRejectedValueOnce(
				new ConnectError(
					"external site returned 429",
					Code.ResourceExhausted,
					new Headers({ "X-Alt-Failure-Scope": "host" }),
				),
			);
			prefetcher.triggerPrefetch(
				[
					makeFeed("0", "https://example.com/active"),
					makeFeed("1", "https://dev.to/article-a"),
				],
				0,
				1,
			);
			await vi.advanceTimersByTimeAsync(600);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			mockedGetContent.mockResolvedValue(body("<p>Free host</p>"));

			// dev.to is cooling down, zenn.dev is not. Lookahead order puts
			// zenn on the second rung, behind 500ms of waiting for a request
			// the cooldown will refuse to send at all.
			prefetcher.triggerPrefetch(
				[
					makeFeed("0", "https://example.com/active"),
					makeFeed("1", "https://dev.to/article-b"),
					makeFeed("2", "https://zenn.dev/article-c"),
				],
				0,
				2,
			);
			await vi.advanceTimersByTimeAsync(600);

			expect(mockedGetContent).toHaveBeenCalledTimes(2);
			expect(mockedGetContent).toHaveBeenLastCalledWith(
				"https://zenn.dev/article-c",
			);
		});

		it("gives the first rung to a host with nothing in flight", async () => {
			// A request to dev.to that never settles: the per-host chain means
			// anything queued behind it waits for a slot that is not free.
			mockedGetContent.mockImplementationOnce(
				() => new Promise<FeedContentOnTheFlyResponse>(() => {}),
			);
			void prefetcher.ensureContent("https://dev.to/in-flight");
			await vi.advanceTimersByTimeAsync(0);
			expect(mockedGetContent).toHaveBeenCalledTimes(1);

			mockedGetContent.mockResolvedValue(body("<p>Free host</p>"));
			prefetcher.triggerPrefetch(
				[
					makeFeed("0", "https://example.com/active"),
					makeFeed("1", "https://dev.to/queued"),
					makeFeed("2", "https://zenn.dev/free"),
				],
				0,
				2,
			);
			await vi.advanceTimersByTimeAsync(600);

			expect(mockedGetContent).toHaveBeenCalledTimes(2);
			expect(mockedGetContent).toHaveBeenLastCalledWith(
				"https://zenn.dev/free",
			);
		});

		it("keeps lookahead order as the tiebreak when every host is free", async () => {
			mockedGetContent.mockResolvedValue(body("<p>Body</p>"));

			prefetcher.triggerPrefetch(
				[
					makeFeed("0", "https://example.com/active"),
					makeFeed("1", "https://a.example/1"),
					makeFeed("2", "https://b.example/2"),
					makeFeed("3", "https://c.example/3"),
				],
				0,
				3,
			);
			await vi.advanceTimersByTimeAsync(2_000);

			expect(mockedGetContent.mock.calls.map((call) => call[0])).toEqual([
				"https://a.example/1",
				"https://b.example/2",
				"https://c.example/3",
			]);
		});

		it("reorders without fetching more than the window holds", async () => {
			// The hard constraint: this is a permutation, never an addition.
			mockedGetContent.mockRejectedValueOnce(
				new ConnectError(
					"external site returned 429",
					Code.ResourceExhausted,
					new Headers({ "X-Alt-Failure-Scope": "host" }),
				),
			);
			prefetcher.triggerPrefetch(
				[
					makeFeed("0", "https://example.com/active"),
					makeFeed("1", "https://dev.to/article-a"),
				],
				0,
				1,
			);
			await vi.advanceTimersByTimeAsync(600);

			mockedGetContent.mockResolvedValue(body("<p>Body</p>"));
			prefetcher.triggerPrefetch(
				[
					makeFeed("0", "https://example.com/active"),
					makeFeed("1", "https://dev.to/article-b"),
					makeFeed("2", "https://zenn.dev/article-c"),
					makeFeed("3", "https://zenn.dev/article-d"),
				],
				0,
				3,
			);
			await vi.advanceTimersByTimeAsync(5_000);

			// dev.to/article-b is still inside its host's cooldown, so the two
			// zenn articles are the only ones that go out.
			const urls = mockedGetContent.mock.calls.slice(1).map((call) => call[0]);
			expect(urls).toEqual([
				"https://zenn.dev/article-c",
				"https://zenn.dev/article-d",
			]);
		});
	});

	describe("eviction", () => {
		it("evicts the oldest entry once the cache exceeds its cap", () => {
			for (let i = 0; i < 31; i++) {
				prefetcher.seedCache(
					`https://example.com/feed-${i}`,
					`<p>Feed ${i}</p>`,
					`art-${i}`,
					`https://og/${i}.png`,
				);
			}

			expect(
				prefetcher.getCachedContent("https://example.com/feed-0"),
			).toBeNull();
			expect(prefetcher.getCachedContent("https://example.com/feed-30")).toBe(
				"<p>Feed 30</p>",
			);
		});

		it("evicts the article id alongside the body it belongs to", () => {
			for (let i = 0; i < 31; i++) {
				prefetcher.seedCache(
					`https://example.com/feed-${i}`,
					`<p>Feed ${i}</p>`,
					`art-${i}`,
					`https://og/${i}.png`,
				);
			}

			// articleIdCache grew unbounded because eviction only pruned
			// contentCache and ogImageCache.
			expect(
				prefetcher.getCachedArticleId("https://example.com/feed-0"),
			).toBeNull();
		});
	});
});

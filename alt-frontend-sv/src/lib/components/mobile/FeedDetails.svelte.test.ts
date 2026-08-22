import { page } from "@vitest/browser/context";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";
import type {
	FeedContentOnTheFlyResponse,
	FetchArticleSummaryResponse,
	SummarizeArticleResponse,
} from "$lib/api/client";
import FeedDetails from "./FeedDetails.svelte";

// Mock API client functions
vi.mock("$lib/api/client", () => ({
	getFeedContentOnTheFlyClient: vi.fn(() =>
		Promise.resolve({
			content: "<p>Full article content here.</p>",
			article_id: "article-123",
		}),
	),
	getArticleSummaryClient: vi.fn(() =>
		Promise.resolve({
			matched_articles: [{ source_id: "article-123" }],
		}),
	),
	summarizeArticleClient: vi.fn(() =>
		Promise.resolve({
			success: true,
			summary: "This is a test summary.",
		}),
	),
	registerFavoriteFeedClient: vi.fn(() => Promise.resolve({ message: "ok" })),
	archiveContentClient: vi.fn(() => Promise.resolve({})),
}));

// Mock Connect RPC functions
vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	streamSummarizeWithAbortAdapter: vi.fn(
		(
			_transport: unknown,
			_options: unknown,
			_updateState: unknown,
			_rendererOptions: unknown,
			onComplete?: (result: unknown) => void,
			_onError?: (error: Error) => void,
		) => {
			if (onComplete) {
				onComplete({});
			}
			return new AbortController();
		},
	),
}));

// Mock $app/environment
vi.mock("$app/environment", () => ({
	browser: true,
}));

const testFeedURL = "https://example.com/test-article";
const testFeedTitle = "Test Article Title";

// The happy-path payloads the vi.mock factory above installs, spelled out with
// their full types. vi.clearAllMocks() clears recorded calls but keeps
// implementations, so a mockRejectedValue (or a mockImplementation) set by one
// test leaks into every test after it; restoreHappyPathMocks() puts each test
// back on a known footing instead of leaving it to depend on file order.
const okContent: FeedContentOnTheFlyResponse = {
	content: "<p>Full article content here.</p>",
	article_id: "article-123",
	og_image_url: "",
	og_image_proxy_url: "",
};

const okSummary: FetchArticleSummaryResponse = {
	matched_articles: [
		{
			article_url: "",
			title: "",
			content: "",
			content_type: "",
			published_at: "",
			fetched_at: "",
			source_id: "article-123",
		},
	],
	total_matched: 1,
	requested_count: 1,
};

const okSummarize: SummarizeArticleResponse = {
	success: true,
	summary: "This is a test summary.",
	article_id: "article-123",
	feed_url: testFeedURL,
};

async function restoreHappyPathMocks() {
	const {
		getFeedContentOnTheFlyClient,
		getArticleSummaryClient,
		summarizeArticleClient,
	} = await import("$lib/api/client");
	vi.mocked(getFeedContentOnTheFlyClient).mockResolvedValue(okContent);
	vi.mocked(getArticleSummaryClient).mockResolvedValue(okSummary);
	vi.mocked(summarizeArticleClient).mockResolvedValue(okSummarize);

	// $lib/connect leaks the same way: the mockImplementation the summary tests
	// install survives vi.clearAllMocks(), so restore the factory's default
	// (complete immediately, no chunks, no error) here too. Parameters are left
	// to contextual typing so this stays pinned to the real adapter signature.
	const { streamSummarizeWithAbortAdapter } = await import("$lib/connect");
	vi.mocked(streamSummarizeWithAbortAdapter).mockImplementation(
		(_transport, _options, _updateState, _rendererOptions, onComplete) => {
			onComplete?.({
				chunkCount: 0,
				totalLength: 0,
				hasReceivedData: false,
				articleId: "article-123",
				wasCached: false,
			});
			return new AbortController();
		},
	);
}

describe("FeedDetails Alt-Paper compliance", () => {
	beforeEach(async () => {
		vi.clearAllMocks();
		await restoreHappyPathMocks();
	});

	it("renders Show Details button when showButton is true", async () => {
		render(FeedDetails, {
			props: {
				feedURL: testFeedURL,
				feedTitle: testFeedTitle,
				showButton: true,
			},
		});

		await expect.element(page.getByText("Show Details")).toBeInTheDocument();
	});

	it("does NOT render Archive button when open", async () => {
		render(FeedDetails, {
			props: {
				feedURL: testFeedURL,
				feedTitle: testFeedTitle,
				open: true,
				onOpenChange: vi.fn(),
				showButton: false,
			},
		});

		await new Promise((resolve) => setTimeout(resolve, 300));

		const archiveEl = page.getByText("Archive");
		await expect.element(archiveEl).not.toBeInTheDocument();
	});

	it("renders Favorite button when open", async () => {
		render(FeedDetails, {
			props: {
				feedURL: testFeedURL,
				feedTitle: testFeedTitle,
				open: true,
				onOpenChange: vi.fn(),
				showButton: false,
			},
		});

		await new Promise((resolve) => setTimeout(resolve, 300));

		await expect
			.element(page.getByRole("button", { name: /favorite/i }))
			.toBeInTheDocument();
	});
});

describe("FeedDetails Android layout", () => {
	beforeEach(async () => {
		vi.clearAllMocks();
		await restoreHappyPathMocks();
	});

	it("uses dvh (not vh) for bottom sheet height to avoid Android toolbar clip", async () => {
		render(FeedDetails, {
			props: {
				feedURL: testFeedURL,
				feedTitle: testFeedTitle,
				open: true,
				onOpenChange: vi.fn(),
				showButton: false,
			},
		});

		await new Promise((resolve) => setTimeout(resolve, 300));

		const sheet = document.querySelector<HTMLElement>(
			'[data-slot="sheet-content"]',
		);
		if (!sheet) throw new Error("sheet-content not rendered");
		expect(sheet.className).toContain("h-[85dvh]");
		expect(sheet.className).not.toMatch(/h-\[85vh\]/);
	});

	it(".sheet-title clamps long title to avoid overlap with absolute close button", async () => {
		const longTitle =
			"This is an extremely long article title that would normally overflow the sheet header area and collide with the close X button in the top right corner of the sheet on a narrow Android viewport";

		render(FeedDetails, {
			props: {
				feedURL: testFeedURL,
				feedTitle: longTitle,
				open: true,
				onOpenChange: vi.fn(),
				showButton: false,
			},
		});

		await new Promise((resolve) => setTimeout(resolve, 300));

		const title = document.querySelector<HTMLElement>(".sheet-title");
		if (!title) throw new Error("sheet-title not rendered");
		const computed = window.getComputedStyle(title);
		expect(computed.minWidth).toBe("0px");
		expect(computed.overflow).toBe("hidden");
		expect(computed.webkitLineClamp).toBe("2");
	});

	it("sheet header reserves space for close button via padding-inline-end", async () => {
		render(FeedDetails, {
			props: {
				feedURL: testFeedURL,
				feedTitle: testFeedTitle,
				open: true,
				onOpenChange: vi.fn(),
				showButton: false,
			},
		});

		await new Promise((resolve) => setTimeout(resolve, 300));

		const header = document.querySelector<HTMLElement>(
			'[data-slot="sheet-header"]',
		);
		if (!header) throw new Error("sheet-header not rendered");
		const computed = window.getComputedStyle(header);
		const paddingRight = Number.parseFloat(computed.paddingRight);
		expect(paddingRight).toBeGreaterThanOrEqual(40);
	});
});

describe("FeedDetails retry", () => {
	beforeEach(async () => {
		vi.clearAllMocks();
		await restoreHappyPathMocks();
	});

	describe("content fetch retry", () => {
		it("shows error with role='alert' when content fetch fails", async () => {
			const { getFeedContentOnTheFlyClient, getArticleSummaryClient } =
				await import("$lib/api/client");
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				new Error("Server error"),
			);
			vi.mocked(getArticleSummaryClient).mockRejectedValue(
				new Error("Server error"),
			);

			render(FeedDetails, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			// Wait for fetch to fail
			await new Promise((resolve) => setTimeout(resolve, 500));

			// Error should be displayed (component shows error via
			// RenderFeedDetails), in the wording every surface now shares.
			const container = page.getByText("Source content unavailable.");
			await expect.element(container).toBeInTheDocument();

			// ...and announced, which is what this test's name has always claimed.
			await expect.element(page.getByRole("alert")).toBeInTheDocument();
		});

		// "Server error" is not transient per $lib/utils/errorClassification, so
		// it must not buy a retry at all. Pins the classification, and rules out
		// a "bound" implemented as a fixed sleep that still hammers the API.
		it("does not retry a permanent content failure", async () => {
			const { getFeedContentOnTheFlyClient, getArticleSummaryClient } =
				await import("$lib/api/client");
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				new Error("Server error"),
			);
			vi.mocked(getArticleSummaryClient).mockRejectedValue(
				new Error("Server error"),
			);

			render(FeedDetails, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			await expect
				.element(page.getByText("Source content unavailable."))
				.toBeInTheDocument();

			expect(getFeedContentOnTheFlyClient).toHaveBeenCalledTimes(1);
			expect(getArticleSummaryClient).toHaveBeenCalledTimes(1);

			// ...and the alert says so. The stripe must report the attempts
			// actually spent, not MAX_CONTENT_ATTEMPTS — announcing the ceiling
			// here would tell the user "stopped after 2 attempts" after one.
			// The trailing "." is load-bearing: it rules out "1 attempts".
			await expect
				.element(page.getByText(/stopped after 1 attempt\./i))
				.toBeInTheDocument();
		});

		// Regression pin for the unbounded retry loop. Before the fix the
		// auto-fetch $effect depended on the state fetchData() writes, so a
		// failed fetch re-armed it forever: this spec file killed its own
		// browser page at ~4,000 console.error/sec and reported neither pass
		// nor fail for any of its tests.
		//
		// A transient error is the case that legitimately retries, so it is the
		// one that has to be bounded. MAX_CONTENT_ATTEMPTS is 2 (one attempt
		// plus the single entry in CONTENT_RETRY_BACKOFFS_MS = 500ms), which is
		// the same budget SwipeFeedCard applies to the same endpoints.
		it("stops retrying a transient content failure after a bounded number of attempts", async () => {
			const { getFeedContentOnTheFlyClient, getArticleSummaryClient } =
				await import("$lib/api/client");
			// "network" is classified transient by isTransientError.
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				new Error("network error"),
			);
			vi.mocked(getArticleSummaryClient).mockRejectedValue(
				new Error("network error"),
			);

			render(FeedDetails, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			// One 500ms backoff, so the terminal state lands a little after 0.5s.
			await new Promise((resolve) => setTimeout(resolve, 900));

			await expect
				.element(page.getByText(/stopped after 2 attempts\./i))
				.toBeInTheDocument();

			expect(getFeedContentOnTheFlyClient).toHaveBeenCalledTimes(2);
			expect(getArticleSummaryClient).toHaveBeenCalledTimes(2);

			// And it stays stopped: no further attempt is made once the budget is
			// spent. This is the assertion the old component could never satisfy.
			await new Promise((resolve) => setTimeout(resolve, 1500));
			expect(getFeedContentOnTheFlyClient).toHaveBeenCalledTimes(2);
			expect(getArticleSummaryClient).toHaveBeenCalledTimes(2);
		});

		// The unexpected-error path (a client that throws synchronously rather
		// than rejecting) is the one branch that can reach the terminal state
		// without an attempt having recorded itself. It must still be terminal,
		// and must still describe itself truthfully.
		it("reports one attempt and stops when the client throws synchronously", async () => {
			const { getFeedContentOnTheFlyClient, getArticleSummaryClient } =
				await import("$lib/api/client");
			vi.mocked(getArticleSummaryClient).mockImplementation(() => {
				throw new Error("synchronous boom");
			});
			vi.mocked(getFeedContentOnTheFlyClient).mockImplementation(() => {
				throw new Error("synchronous boom");
			});

			render(FeedDetails, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			await expect
				.element(page.getByText(/stopped after 1 attempt\./i))
				.toBeInTheDocument();

			await new Promise((resolve) => setTimeout(resolve, 600));
			expect(getArticleSummaryClient).toHaveBeenCalledTimes(1);
		});

		// A bounded retry still sleeps between attempts, so it can outlive the
		// component that started it. Unmounting must invalidate it: otherwise
		// navigating away mid-backoff still spends the rest of the budget on
		// requests nobody is waiting for, and writes state on a dead instance.
		it("abandons a pending retry when the component is destroyed", async () => {
			const { getFeedContentOnTheFlyClient, getArticleSummaryClient } =
				await import("$lib/api/client");
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				new Error("network error"),
			);
			vi.mocked(getArticleSummaryClient).mockRejectedValue(
				new Error("network error"),
			);

			const { unmount } = render(FeedDetails, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			// The first attempt has failed and the retry is parked in its 500ms
			// backoff by now.
			await new Promise((resolve) => setTimeout(resolve, 100));
			expect(getFeedContentOnTheFlyClient).toHaveBeenCalledTimes(1);

			unmount();

			// Well past the backoff: the retry must never have woken up.
			await new Promise((resolve) => setTimeout(resolve, 1000));
			expect(getFeedContentOnTheFlyClient).toHaveBeenCalledTimes(1);
			expect(getArticleSummaryClient).toHaveBeenCalledTimes(1);
		});

		it("offers a user-initiated reload once the automatic retries are exhausted", async () => {
			const { getFeedContentOnTheFlyClient, getArticleSummaryClient } =
				await import("$lib/api/client");
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				new Error("Server error"),
			);
			vi.mocked(getArticleSummaryClient).mockRejectedValue(
				new Error("Server error"),
			);

			render(FeedDetails, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			const reload = page.getByRole("button", { name: /reload article/i });
			await expect.element(reload).toBeInTheDocument();

			// The upstream recovers; the user's explicit retry must reach it.
			vi.mocked(getFeedContentOnTheFlyClient).mockResolvedValue({
				...okContent,
				content: "<p>Recovered article content.</p>",
			});

			await reload.click();

			await expect
				.element(page.getByText(/recovered article content/i))
				.toBeInTheDocument();
			await expect.element(reload).not.toBeInTheDocument();
			expect(getFeedContentOnTheFlyClient).toHaveBeenCalledTimes(2);
		});

		// Bounding the retry put backoff sleeps on the fetch path, so the sheet
		// must not be held closed behind it: "Show Details" opens first and the
		// fetch runs underneath the sheet's own loading state. Pinned with a
		// fetch that never settles — the strongest form of "slow".
		it("opens the sheet immediately instead of waiting for the content fetch", async () => {
			const { getFeedContentOnTheFlyClient, getArticleSummaryClient } =
				await import("$lib/api/client");
			const neverSettles = <T>() => new Promise<T>(() => {});
			vi.mocked(getFeedContentOnTheFlyClient).mockImplementation(() =>
				neverSettles<FeedContentOnTheFlyResponse>(),
			);
			vi.mocked(getArticleSummaryClient).mockImplementation(() =>
				neverSettles<FetchArticleSummaryResponse>(),
			);

			render(FeedDetails, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					showButton: true,
				},
			});

			await page.getByRole("button", { name: /show details/i }).click();

			// The sheet's title only exists once the sheet is open.
			await expect.element(page.getByText(testFeedTitle)).toBeInTheDocument();

			// ...and the fetch really is running underneath, exactly once — the
			// open must not have been bought by skipping the fetch, nor may the
			// auto-fetch effect duplicate the request the click already started.
			expect(getFeedContentOnTheFlyClient).toHaveBeenCalledTimes(1);
			expect(getArticleSummaryClient).toHaveBeenCalledTimes(1);
		});
	});

	describe("honest content states", () => {
		it("says what it is doing while the body is in flight", async () => {
			const { getFeedContentOnTheFlyClient, getArticleSummaryClient } =
				await import("$lib/api/client");
			const neverSettles = <T>() => new Promise<T>(() => {});
			vi.mocked(getFeedContentOnTheFlyClient).mockImplementation(() =>
				neverSettles<FeedContentOnTheFlyResponse>(),
			);
			vi.mocked(getArticleSummaryClient).mockImplementation(() =>
				neverSettles<FetchArticleSummaryResponse>(),
			);

			render(FeedDetails, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			await expect
				.element(page.getByText("Fetching the full article…"))
				.toBeInTheDocument();
		});

		it("offers the original site alongside the reload once it is terminal", async () => {
			// NN/g heuristic 9: "Stopped after 1 attempt." states the problem.
			// A reload the reader can press and a way out to the publisher are
			// what make it a remedy rather than a verdict.
			const { getFeedContentOnTheFlyClient, getArticleSummaryClient } =
				await import("$lib/api/client");
			vi.mocked(getFeedContentOnTheFlyClient).mockRejectedValue(
				new Error("Server error"),
			);
			vi.mocked(getArticleSummaryClient).mockRejectedValue(
				new Error("Server error"),
			);

			render(FeedDetails, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			await expect
				.element(page.getByTestId("read-original-link"))
				.toHaveAttribute("href", testFeedURL);
		});

		it("states the shared wording when the body comes back empty", async () => {
			// A successful response carrying `content: ""` is a state, not a
			// falsy no-op (ADR-000581), and it reads the same here as anywhere.
			const { getFeedContentOnTheFlyClient, getArticleSummaryClient } =
				await import("$lib/api/client");
			vi.mocked(getFeedContentOnTheFlyClient).mockResolvedValue({
				content: "",
				article_id: "",
			} as FeedContentOnTheFlyResponse);
			vi.mocked(getArticleSummaryClient).mockResolvedValue({
				matched_articles: [],
			} as unknown as FetchArticleSummaryResponse);

			render(FeedDetails, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			await expect
				.element(page.getByText("Source content unavailable."))
				.toBeInTheDocument();
		});
	});

	describe("summary retry", () => {
		it("shows summary error with role='alert' when summarization fails", async () => {
			const { streamSummarizeWithAbortAdapter } = await import("$lib/connect");
			const { summarizeArticleClient } = await import("$lib/api/client");

			// Summarization only fails once BOTH paths fail. A stream error with
			// no partial data deliberately falls back to the legacy REST endpoint
			// (see handleSummarize, and the same fallback in the desktop
			// FeedDetailModal and useSummarize), so leaving the legacy mock on its
			// happy path meant this test never reached the error state it names.
			vi.mocked(summarizeArticleClient).mockRejectedValue(
				new Error("500 Internal Server Error"),
			);

			// Make stream fail with non-transient error
			vi.mocked(streamSummarizeWithAbortAdapter).mockImplementation(
				(
					_transport: unknown,
					_options: unknown,
					_onChunk: unknown,
					_rendererOptions: unknown,
					_onComplete?: unknown,
					onError?: (error: Error) => void,
				) => {
					setTimeout(() => {
						onError?.(new Error("500 Internal Server Error"));
					}, 10);
					return new AbortController();
				},
			);

			render(FeedDetails, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			// Wait for initial data load
			await new Promise((resolve) => setTimeout(resolve, 300));

			// Click Summarize button. NOT /summary/i: that locator was copied
			// from the SwipeFeedCard specs, where the card's button really is
			// labelled "Summary". This sheet — like its desktop counterparts
			// FeedDetailModal and ArticleDetailPanel — labels it "Summarize",
			// which /summary/i never matched. The spec simply never ran, so the
			// mismatch went unnoticed; the product label is the correct one.
			const summaryButton = page.getByRole("button", { name: /summarize/i });
			await summaryButton.click();

			// Wait for error
			await new Promise((resolve) => setTimeout(resolve, 500));

			// Error alert should appear
			await expect.element(page.getByRole("alert")).toBeInTheDocument();
		});

		it("summary button shows 'Try again' after error", async () => {
			const { streamSummarizeWithAbortAdapter } = await import("$lib/connect");
			const { summarizeArticleClient } = await import("$lib/api/client");

			// See the note above: the legacy fallback has to fail too.
			vi.mocked(summarizeArticleClient).mockRejectedValue(
				new Error("500 Internal Server Error"),
			);

			vi.mocked(streamSummarizeWithAbortAdapter).mockImplementation(
				(
					_transport: unknown,
					_options: unknown,
					_onChunk: unknown,
					_rendererOptions: unknown,
					_onComplete?: unknown,
					onError?: (error: Error) => void,
				) => {
					setTimeout(() => {
						onError?.(new Error("500 Internal Server Error"));
					}, 10);
					return new AbortController();
				},
			);

			render(FeedDetails, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			await new Promise((resolve) => setTimeout(resolve, 300));

			// See the note above: the idle label is "Summarize", not "Summary".
			const summaryButton = page.getByRole("button", { name: /summarize/i });
			await summaryButton.click();

			await new Promise((resolve) => setTimeout(resolve, 500));

			// Button should show "Try again"
			await expect
				.element(page.getByRole("button", { name: /try again/i }))
				.toBeInTheDocument();
		});

		// The other half of the two tests above: a stream error alone is NOT a
		// failed summarization, because the legacy endpoint is meant to cover
		// it. Pinned so the fallback is not mistaken for dead code later.
		it("falls back to the legacy endpoint when the stream errors, without surfacing an error", async () => {
			const { streamSummarizeWithAbortAdapter } = await import("$lib/connect");
			const { summarizeArticleClient } = await import("$lib/api/client");

			vi.mocked(streamSummarizeWithAbortAdapter).mockImplementation(
				(
					_transport: unknown,
					_options: unknown,
					_onChunk: unknown,
					_rendererOptions: unknown,
					_onComplete?: unknown,
					onError?: (error: Error) => void,
				) => {
					setTimeout(() => {
						onError?.(new Error("500 Internal Server Error"));
					}, 10);
					return new AbortController();
				},
			);

			render(FeedDetails, {
				props: {
					feedURL: testFeedURL,
					feedTitle: testFeedTitle,
					open: true,
					onOpenChange: vi.fn(),
					showButton: false,
				},
			});

			await new Promise((resolve) => setTimeout(resolve, 300));

			await page.getByRole("button", { name: /summarize/i }).click();

			await expect
				.element(page.getByText("This is a test summary."))
				.toBeInTheDocument();

			expect(summarizeArticleClient).toHaveBeenCalledTimes(1);
			await expect.element(page.getByRole("alert")).not.toBeInTheDocument();
		});
	});
});

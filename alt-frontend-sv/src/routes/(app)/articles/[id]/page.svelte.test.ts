import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { page as testPage } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { goto } from "$app/navigation";

const DEFAULT_PAGE_URL =
	"http://localhost/articles/test-123?url=https%3A%2F%2Fexample.com%2Farticle&title=Test+Article";

// Read through a getter rather than captured at mock time: the masthead cases
// below vary the query string, and a frozen URL would make every one of them
// exercise the same handoff.
let currentArticleId = $state("test-123");
let currentPageURL = $state(new URL(DEFAULT_PAGE_URL));

vi.mock("$app/state", () => ({
	page: {
		get params() {
			return { id: currentArticleId };
		},
		get url() {
			return currentPageURL;
		},
	},
}));

// +page.svelte imports `goto` and useTrailOutcome imports `beforeNavigate`
// from `$app/navigation`. `$app/navigation` re-exports via `export *`, which
// Vitest `{ spy: true }` automocking cannot wrap, so we provide an explicit
// factory mock with both bindings (`goto` stubbed to prevent real browser
// navigation mid-test, and `beforeNavigate` stubbed). The client and connect
// modules below have concrete named exports and use `{ spy: true }`.
vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
	beforeNavigate: vi.fn(),
}));
vi.mock("#lib/api/client/knowledge_home.js", { spy: true });
vi.mock("#lib/connect/knowledge_trail.js", { spy: true });
vi.mock("#lib/connect/reading.js", { spy: true });
vi.mock("#lib/connect/transport-client.js", { spy: true });

import { trackHomeActionClient } from "#lib/api/client/knowledge_home.js";
import {
	emitTrailOutcome,
	getItemBranches,
	resolveBranch,
} from "#lib/connect/knowledge_trail.js";
import {
	getReadTodayQueue,
	QueueArticleOutcome,
	queueArticle,
	recordReadingEvents,
	unqueueArticle,
} from "#lib/connect/reading.js";
import { createClientTransport } from "#lib/connect/transport-client.js";
import {
	flushReadingEvents,
	ReadingSurface,
} from "#lib/reading/instrumentation.js";
import { readTodayQueue } from "#lib/reading/read-today-queue.svelte.js";

const mockGetFeedContent = vi.fn();
const mockGetArticleSourceURL = vi
	.fn()
	.mockResolvedValue({ url: "", title: "" });
vi.mock("#lib/api/client/articles.js", async (importOriginal) => {
	const actual = await importOriginal<Record<string, unknown>>();
	return {
		...actual,
		getFeedContentOnTheFlyClient: (...args: unknown[]) =>
			mockGetFeedContent(...args),
		getArticleSourceURLClient: (...args: unknown[]) =>
			mockGetArticleSourceURL(...args),
	};
});

const mockTrackHomeActionClient = vi.mocked(trackHomeActionClient);
const mockGetItemBranches = vi.mocked(getItemBranches);
const mockResolveBranch = vi.mocked(resolveBranch);
const mockEmitTrailOutcome = vi.mocked(emitTrailOutcome);
const mockRecordReadingEvents = vi.mocked(recordReadingEvents);

beforeEach(async () => {
	readTodayQueue.reset();
	mockRecordReadingEvents.mockResolvedValue({
		accepted: 0,
		duplicates: 0,
		readArticleIds: [],
	});
	await flushReadingEvents();
	mockRecordReadingEvents.mockClear();
	mockRecordReadingEvents.mockResolvedValue({
		accepted: 1,
		duplicates: 0,
		readArticleIds: [],
	});
	vi.mocked(getReadTodayQueue).mockResolvedValue({
		items: [],
		cap: 7,
		readCountSince: 0,
	});
	vi.mocked(queueArticle).mockResolvedValue({
		outcome: QueueArticleOutcome.QUEUED,
		items: [],
		cap: 7,
	});
	vi.mocked(unqueueArticle).mockResolvedValue(undefined);
	mockTrackHomeActionClient.mockResolvedValue(undefined);
	mockGetItemBranches.mockResolvedValue([]);
	mockResolveBranch.mockResolvedValue(undefined);
	mockEmitTrailOutcome.mockResolvedValue(undefined);
	vi.mocked(createClientTransport).mockReturnValue(
		{} as ReturnType<typeof createClientTransport>,
	);
});

const mockSpeechPlay = vi.fn();
const mockSpeechStop = vi.fn();
vi.mock("#lib/hooks/useSummarySpeech.svelte.js", () => ({
	createSummarySpeech: () => ({
		state: "idle",
		play: mockSpeechPlay,
		stop: mockSpeechStop,
	}),
}));

const mockSummarizerReset = vi.fn();
let summarizerOverride: Partial<{
	summary: string | null;
	isSummarizing: boolean;
	summaryError: string | null;
	buttonState: "idle" | "loading" | "error" | "success";
}> = {};
vi.mock("#lib/hooks/useSummarize.svelte.js", () => ({
	useSummarize: () => ({
		summary: null,
		isSummarizing: false,
		summaryError: null,
		buttonState: "idle" as const,
		summarize: vi.fn(),
		abort: vi.fn(),
		reset: mockSummarizerReset,
		...summarizerOverride,
	}),
}));

import Page from "./+page.svelte";

function renderPage() {
	return render(Page);
}

const connectError = (
	code: Code,
	headers: Record<string, string> = {},
): ConnectError =>
	new ConnectError(
		"Service temporarily unavailable due to circuit breaker",
		code,
		headers,
	);

describe("Article page fetch button", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(goto).mockImplementation(async () => {});
		summarizerOverride = {};
		currentArticleId = "test-123";
		currentPageURL = new URL(DEFAULT_PAGE_URL);
	});

	it("shows disabled Fetching... button while loading", async () => {
		mockGetFeedContent.mockReturnValue(new Promise(() => {}));

		renderPage();

		const button = testPage.getByTestId("fetch-button");
		await expect.element(button).toBeInTheDocument();
		await expect.element(button).toHaveTextContent("Fetching...");
		await expect.element(button).toBeDisabled();
	});

	it("shows Re-fetch button after successful fetch", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Article content</p>",
			article_id: "a1",
		});

		renderPage();

		const button = testPage.getByTestId("fetch-button");
		await expect.element(button).toHaveTextContent("Re-fetch");
	});

	it("shows destructive Try again button on fetch error", async () => {
		mockGetFeedContent.mockRejectedValueOnce(new Error("Network error"));

		renderPage();

		const button = testPage.getByTestId("fetch-button");
		await expect.element(button).toHaveTextContent("Try again");
	});

	it("calls getFeedContentOnTheFlyClient with forceRefresh on re-fetch", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Content</p>",
			article_id: "a1",
		});
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>New content</p>",
			article_id: "a1",
		});

		renderPage();

		const button = testPage.getByTestId("fetch-button");
		await expect.element(button).toHaveTextContent("Re-fetch");

		await button.click();

		expect(mockGetFeedContent).toHaveBeenCalledTimes(2);
		expect(mockGetFeedContent).toHaveBeenLastCalledWith(
			"https://example.com/article",
			{ forceRefresh: true },
		);
	});

	it("resets summarizer on re-fetch", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Content</p>",
			article_id: "a1",
		});
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>New content</p>",
			article_id: "a1",
		});

		renderPage();

		const button = testPage.getByTestId("fetch-button");
		await expect.element(button).toHaveTextContent("Re-fetch");

		await button.click();

		expect(mockSummarizerReset).toHaveBeenCalled();
	});
});

describe("Article page Alt-Paper mobile layout", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(goto).mockImplementation(async () => {});
		summarizerOverride = {};
		currentArticleId = "test-123";
		currentPageURL = new URL(DEFAULT_PAGE_URL);
	});

	it("renders editorial masthead with page-kicker role and article title", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Article body</p>",
			article_id: "a1",
		});

		renderPage();

		const masthead = testPage.getByTestId("article-masthead");
		await expect.element(masthead).toBeInTheDocument();
		await expect.element(masthead).toHaveAttribute("data-role", "page-kicker");
		await expect.element(masthead).toHaveTextContent("Test Article");
	});

	it("uses Alt-Paper surface tokens without rounded-lg/bg-white violations", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Article body</p>",
			article_id: "a1",
		});

		const { container } = renderPage();

		await expect
			.element(testPage.getByTestId("article-content-surface"))
			.toBeInTheDocument();

		const offenders = container.querySelectorAll(
			".rounded-lg, .rounded-xl, .rounded-2xl, .bg-white",
		);
		expect(offenders.length).toBe(0);
	});

	it("renders AI SUMMARY kicker when summary is present", async () => {
		summarizerOverride = {
			summary: "Condensed bullet list",
			buttonState: "success",
		};
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Article body</p>",
			article_id: "a1",
		});

		renderPage();

		const summary = testPage.getByTestId("ai-summary");
		await expect.element(summary).toBeInTheDocument();
		await expect.element(summary).toHaveTextContent("AI SUMMARY");
		await expect.element(summary).toHaveTextContent("Condensed bullet list");
	});

	it("flags an interrupted stream alongside the partial summary", async () => {
		// A cut stream leaves text on screen. Without the notice the reader
		// cannot tell a truncated summary from a short one.
		summarizerOverride = {
			summary: "Half a sentence before the stream",
			summaryError: "[unknown] missing EndStreamResponse",
			buttonState: "error",
		};
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Article body</p>",
			article_id: "a1",
		});

		renderPage();

		await expect
			.element(testPage.getByTestId("ai-summary"))
			.toHaveTextContent("Half a sentence before the stream");
		await expect
			.element(testPage.getByTestId("summary-interrupted"))
			.toHaveTextContent("Stream interrupted. Summary may be incomplete.");
	});

	it("shows the summarize error on its own when no text arrived", async () => {
		summarizerOverride = {
			summary: null,
			summaryError: "Failed to generate summary",
			buttonState: "error",
		};
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Article body</p>",
			article_id: "a1",
		});

		renderPage();

		const alert = testPage.getByRole("alert");
		await expect.element(alert).toHaveTextContent("SUMMARIZE ERROR");
		await expect.element(alert).toHaveTextContent("Failed to generate summary");
	});

	it("shows no interruption notice for a summary that completed", async () => {
		summarizerOverride = {
			summary: "Condensed bullet list",
			summaryError: null,
			buttonState: "success",
		};
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Article body</p>",
			article_id: "a1",
		});

		const { container } = renderPage();

		await expect
			.element(testPage.getByTestId("ai-summary"))
			.toBeInTheDocument();
		expect(
			container.querySelector('[data-testid="summary-interrupted"]'),
		).toBeNull();
	});

	it("exposes icon-only actions with accessible labels on mobile", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Article body</p>",
			article_id: "a1",
		});

		renderPage();

		await expect
			.element(testPage.getByRole("button", { name: /back to today/i }))
			.toBeInTheDocument();
		await expect
			.element(testPage.getByTestId("fetch-button"))
			.toHaveAttribute("aria-label");
		await expect
			.element(testPage.getByTestId("summarize-button"))
			.toHaveAttribute("aria-label");
		await expect
			.element(testPage.getByRole("link", { name: /open original/i }))
			.toBeInTheDocument();
	});

	it("renders read-aloud control when summary is complete and calls createSummarySpeech", async () => {
		summarizerOverride = {
			summary: "Read aloud summary text.",
			isSummarizing: false,
			summaryError: null,
			buttonState: "success",
		};
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Article body</p>",
			article_id: "a1",
		});

		const { unmount } = renderPage();

		const speechBtn = testPage.getByTestId("summary-speech-button");
		await expect.element(speechBtn).toBeInTheDocument();
		await speechBtn.click();
		expect(mockSpeechPlay).toHaveBeenCalledWith("Read aloud summary text.");

		unmount();
		expect(mockSpeechStop).toHaveBeenCalled();
	});
});

describe("Article page masthead", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(goto).mockImplementation(async () => {});
		summarizerOverride = {};
		currentArticleId = "test-123";
		currentPageURL = new URL(DEFAULT_PAGE_URL);
		mockGetFeedContent.mockReturnValue(new Promise(() => {}));
	});

	it("uses the ?title= handoff when the calling surface supplied one", async () => {
		renderPage();

		await expect
			.element(testPage.getByRole("heading", { level: 1 }))
			.toHaveTextContent("Test Article");
	});

	it("resolves the title from the article id when no ?title= is passed", async () => {
		// The summary-ready notification navigates to /articles/<id> with no
		// query string at all, so the stored title is the only headline
		// available. Deriving one from the URL host shipped Guardian features
		// titled "www.theguardian.com".
		currentPageURL = new URL("http://localhost/articles/test-123");
		mockGetArticleSourceURL.mockResolvedValue({
			url: "https://www.theguardian.com/lifeandstyle/toy-store",
			title: "Working at a toy store was my teen feminist awakening",
		});

		renderPage();

		await expect
			.element(testPage.getByRole("heading", { level: 1 }))
			.toHaveTextContent(
				"Working at a toy store was my teen feminist awakening",
			);
	});

	it("resolves the title when the caller passed a url but no title", async () => {
		// A Knowledge Home card whose projection row has a blank title omits
		// the ?title= parameter while still passing ?url=. Resolution has to be
		// driven by the missing title, not by the missing URL.
		currentPageURL = new URL(
			"http://localhost/articles/test-123?url=https%3A%2F%2Fwww.theguardian.com%2Flifeandstyle%2Ftoy-store",
		);
		mockGetArticleSourceURL.mockResolvedValue({
			url: "https://www.theguardian.com/lifeandstyle/toy-store",
			title: "Working at a toy store was my teen feminist awakening",
		});

		renderPage();

		await expect
			.element(testPage.getByRole("heading", { level: 1 }))
			.toHaveTextContent(
				"Working at a toy store was my teen feminist awakening",
			);
	});

	it("falls back to a neutral label, never the host, when no title exists", async () => {
		currentPageURL = new URL("http://localhost/articles/test-123");
		mockGetArticleSourceURL.mockResolvedValue({
			url: "https://www.theguardian.com/lifeandstyle/toy-store",
			title: "",
		});

		renderPage();

		const heading = testPage.getByRole("heading", { level: 1 });
		await expect.element(heading).toBeInTheDocument();
		await expect.element(heading).not.toHaveTextContent("www.theguardian.com");
	});
});

describe("Article page content states", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(goto).mockImplementation(async () => {});
		summarizerOverride = {};
		currentArticleId = "test-123";
		currentPageURL = new URL(DEFAULT_PAGE_URL);
	});

	it("treats an empty body as a state instead of re-firing the effect forever", async () => {
		// ADR-000581: `articleContent = response.content || null` left BOTH
		// articleContent and contentError null on an empty body, so the
		// auto-fetch $effect's guard passed again the moment isFetching
		// cleared. That is the infinite loop that ADR closed on the desktop
		// modal — it was still open here.
		mockGetFeedContent.mockResolvedValue({ content: "", article_id: "" });

		renderPage();

		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.toBeInTheDocument();
		await new Promise((r) => setTimeout(r, 600));
		expect(mockGetFeedContent).toHaveBeenCalledTimes(1);
	});

	it("says what it is doing while the body is in flight", async () => {
		mockGetFeedContent.mockReturnValue(new Promise(() => {}));

		renderPage();

		await expect
			.element(testPage.getByTestId("article-content-pending"))
			.toHaveTextContent("Fetching the full article");
		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.not.toBeInTheDocument();
	});

	it("offers both remedies, and never the upstream prose, when it is terminal", async () => {
		mockGetFeedContent.mockRejectedValue(connectError(Code.Unavailable));

		renderPage();

		const failed = testPage.getByTestId("article-content-failed");
		await expect
			.element(failed)
			.toHaveTextContent(
				"Source content is temporarily unavailable. Please try again shortly.",
			);
		// ADR-000959 §6: the backend's prose must not reach the reader.
		await expect.element(failed).not.toHaveTextContent("circuit breaker");
		await expect
			.element(testPage.getByTestId("read-original-link"))
			.toHaveAttribute("href", "https://example.com/article");
		await expect
			.element(testPage.getByTestId("retry-content"))
			.toBeInTheDocument();
	});

	it("retries an unstamped ResourceExhausted exactly once, announcing the wait", async () => {
		mockGetFeedContent
			.mockRejectedValueOnce(
				connectError(Code.ResourceExhausted, { "Retry-After": "0" }),
			)
			.mockResolvedValue({
				content: "<p>Arrived on the second attempt.</p>",
				article_id: "a1",
			});

		renderPage();

		await expect
			.element(testPage.getByTestId("article-content-pending"))
			.toHaveTextContent("Retrying");
		await expect
			.element(testPage.getByText("Arrived on the second attempt."))
			.toBeInTheDocument();
		expect(mockGetFeedContent).toHaveBeenCalledTimes(2);
	});

	it("does NOT retry Code.Unavailable — ADR-000959 rejected exactly that", async () => {
		mockGetFeedContent.mockRejectedValue(
			connectError(Code.Unavailable, { "Retry-After": "0" }),
		);

		renderPage();

		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.toBeInTheDocument();
		await new Promise((r) => setTimeout(r, 900));
		expect(mockGetFeedContent).toHaveBeenCalledTimes(1);
	});

	it("does NOT retry a ResourceExhausted stamped as the publisher's own 429", async () => {
		mockGetFeedContent.mockRejectedValue(
			connectError(Code.ResourceExhausted, {
				"Retry-After": "0",
				"X-Alt-Failure-Scope": "host",
			}),
		);

		renderPage();

		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.toBeInTheDocument();
		await new Promise((r) => setTimeout(r, 900));
		expect(mockGetFeedContent).toHaveBeenCalledTimes(1);
	});
});

describe("Article page branch walk and open tracking", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(goto).mockImplementation(async () => {});
		summarizerOverride = {};
		currentArticleId = "art-a";
		currentPageURL = new URL(
			"http://localhost/articles/art-a?url=https%3A%2F%2Fexample.com%2Fa&title=Article+A&trail_proposal=prop-a",
		);
	});

	it("reloads content and end branches, records open once per article visit, and emits dwell outcomes for both branch walks", async () => {
		mockGetFeedContent.mockImplementation((url: string) =>
			Promise.resolve({
				content:
					url === "https://example.com/b"
						? "<p>Second walked article body.</p>"
						: "<p>First walked article body.</p>",
				article_id: url === "https://example.com/b" ? "art-b" : "art-a",
			}),
		);
		mockGetArticleSourceURL.mockResolvedValue({
			url: "https://example.com/b",
			title: "Article B",
		});
		mockGetItemBranches.mockImplementation(
			(_transport: unknown, itemKey: string) =>
				Promise.resolve([
					{
						branchKey: `cluster:u:${itemKey}`,
						anchorItemKey: itemKey,
						relationKind: "cluster",
						why: `Branch from ${itemKey}`,
						evidenceRefs: [{ refId: "rust", label: "rust", kind: "tag" }],
						confidence: "plausible",
						targetItemKey: "article:art-b",
						targetTitle: `Target from ${itemKey}`,
					},
				]),
		);

		const { unmount } = renderPage();

		await expect
			.element(testPage.getByText("First walked article body."))
			.toBeInTheDocument();
		await expect
			.element(testPage.getByText("Branch from article:art-a"))
			.toBeInTheDocument();

		expect(mockTrackHomeActionClient).toHaveBeenCalledTimes(1);
		expect(mockTrackHomeActionClient).toHaveBeenCalledWith(
			"open",
			"article:art-a",
			JSON.stringify({ source: "article_page" }),
		);
		expect(mockGetItemBranches).toHaveBeenCalledTimes(1);
		expect(mockGetItemBranches).toHaveBeenCalledWith(
			expect.anything(),
			"article:art-a",
			2,
		);

		// Walk a branch to /articles/art-b?trail_proposal=prop-b
		currentArticleId = "art-b";
		currentPageURL = new URL(
			"http://localhost/articles/art-b?trail_proposal=prop-b",
		);

		await expect
			.element(testPage.getByText("Second walked article body."))
			.toBeInTheDocument();
		await expect
			.element(testPage.getByText("Branch from article:art-b"))
			.toBeInTheDocument();

		expect(mockTrackHomeActionClient).toHaveBeenCalledTimes(2);
		expect(mockTrackHomeActionClient).toHaveBeenLastCalledWith(
			"open",
			"article:art-b",
			JSON.stringify({ source: "article_page" }),
		);
		expect(mockGetItemBranches).toHaveBeenCalledTimes(2);
		expect(mockGetItemBranches).toHaveBeenLastCalledWith(
			expect.anything(),
			"article:art-b",
			2,
		);

		// Leaving the first article flushed its dwell outcome; unmounting the
		// second article flushes the second branch walk's dwell outcome too.
		expect(mockEmitTrailOutcome).toHaveBeenCalledTimes(1);
		expect(mockEmitTrailOutcome).toHaveBeenCalledWith(
			expect.anything(),
			"prop-a",
			"article:art-a",
			expect.any(BigInt),
		);

		unmount();

		expect(mockEmitTrailOutcome).toHaveBeenCalledTimes(2);
		expect(mockEmitTrailOutcome).toHaveBeenLastCalledWith(
			expect.anything(),
			"prop-b",
			"article:art-b",
			expect.any(BigInt),
		);
	});

	it("ignores a late endBranches response whose anchor no longer matches the current article", async () => {
		let resolveBranchesA!: (
			value: Awaited<ReturnType<typeof getItemBranches>>,
		) => void;
		const slowBranchesAPromise = new Promise<
			Awaited<ReturnType<typeof getItemBranches>>
		>((resolve) => {
			resolveBranchesA = resolve;
		});

		mockGetFeedContent.mockImplementation((url: string) =>
			Promise.resolve({
				content:
					url === "https://example.com/b"
						? "<p>Second walked article body.</p>"
						: "<p>First walked article body.</p>",
				article_id: url === "https://example.com/b" ? "art-b" : "art-a",
			}),
		);
		mockGetArticleSourceURL.mockResolvedValue({
			url: "https://example.com/b",
			title: "Article B",
		});
		mockGetItemBranches.mockImplementation(
			(_transport: unknown, itemKey: string) => {
				if (itemKey === "article:art-a") {
					return slowBranchesAPromise;
				}
				return Promise.resolve([
					{
						branchKey: `cluster:u:${itemKey}`,
						anchorItemKey: itemKey,
						relationKind: "cluster",
						why: `Branch from ${itemKey}`,
						evidenceRefs: [{ refId: "rust", label: "rust", kind: "tag" }],
						confidence: "plausible",
						targetItemKey: "article:art-c",
						targetTitle: `Target from ${itemKey}`,
					},
				]);
			},
		);

		renderPage();

		await expect
			.element(testPage.getByText("First walked article body."))
			.toBeInTheDocument();

		// Navigate to article B while article A's getItemBranches is still in flight
		currentArticleId = "art-b";
		currentPageURL = new URL("http://localhost/articles/art-b");

		await expect
			.element(testPage.getByText("Second walked article body."))
			.toBeInTheDocument();
		await expect
			.element(testPage.getByText("Branch from article:art-b"))
			.toBeInTheDocument();

		// Late arrival of article A's branches must be ignored
		resolveBranchesA([
			{
				branchKey: "cluster:u:article:art-a",
				anchorItemKey: "article:art-a",
				relationKind: "cluster",
				why: "Stale branch from article:art-a",
				evidenceRefs: [{ refId: "rust", label: "rust", kind: "tag" }],
				confidence: "plausible",
				targetItemKey: "article:art-b",
				targetTitle: "Stale target from article:art-a",
			},
		]);
		await slowBranchesAPromise;
		await new Promise((r) => setTimeout(r, 50));

		await expect
			.element(testPage.getByText("Branch from article:art-b"))
			.toBeInTheDocument();
		await expect
			.element(testPage.getByText("Stale branch from article:art-a"))
			.not.toBeInTheDocument();
	});
});

describe("Article page reading session recording", () => {
	const READ_PAGE_ID = "01938e82-7c00-7a7b-9b10-000000000101";

	beforeEach(() => {
		vi.clearAllMocks();
		mockRecordReadingEvents.mockResolvedValue({
			accepted: 1,
			duplicates: 0,
			readArticleIds: [],
		});
		vi.mocked(goto).mockImplementation(async () => {});
		summarizerOverride = {};
		currentArticleId = READ_PAGE_ID;
		currentPageURL = new URL(
			`http://localhost/articles/${READ_PAGE_ID}?url=https%3A%2F%2Fexample.com%2Fread-page&title=Read+Page`,
		);
	});

	it("enqueues surface_viewed and records session_ended with route param articleId on unmount", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Standalone article page body text.</p>",
			article_id: "different-fetched-id",
		});

		const { unmount } = renderPage();

		await expect
			.element(testPage.getByText("Standalone article page body text."))
			.toBeInTheDocument();

		expect(mockRecordReadingEvents).not.toHaveBeenCalled();

		unmount();

		await vi.waitFor(() =>
			expect(mockRecordReadingEvents).toHaveBeenCalledTimes(1),
		);
		const events = mockRecordReadingEvents.mock.calls[0]?.[1] ?? [];
		expect(events.map((e) => e.event?.case)).toContain("surfaceViewed");
		expect(events.map((e) => e.event?.case)).toContain("opened");
		expect(events.map((e) => e.event?.case)).toContain("sessionEnded");
		const opened = events.find((e) => e.event?.case === "opened")?.event?.value;
		expect(opened).toMatchObject({
			articleId: READ_PAGE_ID,
			surface: ReadingSurface.ARTICLE_PAGE,
		});
		const surfaceViewed = events.find((e) => e.event?.case === "surfaceViewed")
			?.event?.value;
		expect(surfaceViewed).toEqual(
			expect.objectContaining({ surface: ReadingSurface.ARTICLE_PAGE }),
		);
		const sessionEnded = events.find((e) => e.event?.case === "sessionEnded")
			?.event?.value;
		expect(sessionEnded).toMatchObject({
			articleId: READ_PAGE_ID,
			surface: ReadingSurface.ARTICLE_PAGE,
		});
		expect(
			(sessionEnded as { bodyLengthChars?: number })?.bodyLengthChars ?? 0,
		).toBeGreaterThan(0);
	});

	it("does not enqueue opened or session_ended when /articles/[id] param is not a UUID", async () => {
		currentArticleId = "not-a-uuid";
		currentPageURL = new URL(
			"http://localhost/articles/not-a-uuid?url=https%3A%2F%2Fexample.com%2Fnon-uuid&title=Non+UUID",
		);
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Non-UUID article page body.</p>",
			article_id: "also-not-a-uuid",
		});

		const { unmount } = renderPage();

		await expect
			.element(testPage.getByText("Non-UUID article page body."))
			.toBeInTheDocument();

		unmount();
		await flushReadingEvents();

		const events = mockRecordReadingEvents.mock.calls.flatMap(
			(call) => call[1] ?? [],
		);
		expect(events.map((e) => e.event?.case)).toContain("surfaceViewed");
		expect(events.map((e) => e.event?.case)).not.toContain("opened");
		expect(events.map((e) => e.event?.case)).not.toContain("sessionEnded");
	});

	it("records surface_viewed and opened without session_ended when content fetch fails", async () => {
		mockGetFeedContent.mockRejectedValueOnce(connectError(Code.Unavailable));

		const { unmount } = renderPage();

		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.toBeInTheDocument();

		unmount();
		await flushReadingEvents();

		const events = mockRecordReadingEvents.mock.calls.flatMap(
			(call) => call[1] ?? [],
		);
		expect(events.map((e) => e.event?.case)).toContain("surfaceViewed");
		expect(events.map((e) => e.event?.case)).toContain("opened");
		expect(events.map((e) => e.event?.case)).not.toContain("sessionEnded");
	});

	it("toggles Read today / In Read today from the standalone article page action cluster", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Standalone queue toggle.</p>",
			article_id: READ_PAGE_ID,
		});
		vi.mocked(queueArticle).mockResolvedValueOnce({
			outcome: QueueArticleOutcome.QUEUED,
			items: [
				{
					articleId: READ_PAGE_ID,
					title: "Read Page",
					url: "https://example.com/read-page",
					sourceName: "example.com",
					imageUrl: "",
					queuedAt: new Date(),
					expiresAt: new Date(Date.now() + 3600_000),
				},
			],
			cap: 7,
		});

		renderPage();

		await expect
			.element(testPage.getByText("Standalone queue toggle."))
			.toBeInTheDocument();

		const toggleBtn = testPage.getByTestId("read-today-toggle-button");
		await expect.element(toggleBtn).toHaveAccessibleName("Read today");
		await toggleBtn.click();

		await expect.element(toggleBtn).toHaveAccessibleName("In Read today");
		expect(queueArticle).toHaveBeenCalledWith(expect.anything(), {
			articleId: READ_PAGE_ID,
			surface: ReadingSurface.ARTICLE_PAGE,
		});
	});
});

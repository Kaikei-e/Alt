import { beforeEach, describe, expect, it, vi } from "vitest";
import { page as testPage } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { goto } from "$app/navigation";

const DEFAULT_PAGE_URL =
	"http://localhost/articles/test-123?url=https%3A%2F%2Fexample.com%2Farticle&title=Test+Article";

// Read through a getter rather than captured at mock time: the masthead cases
// below vary the query string, and a frozen URL would make every one of them
// exercise the same handoff.
let currentPageURL = new URL(DEFAULT_PAGE_URL);

vi.mock("$app/state", () => ({
	page: {
		params: { id: "test-123" },
		get url() {
			return currentPageURL;
		},
	},
}));

// The route reaches $app/navigation by two independent paths: +page.svelte
// imports `goto`, and useTrailOutcome — the trail dwell measurement — imports
// `beforeNavigate` to flush on leave. A factory that enumerates a subset
// replaces the *whole* module, so any binding it omits becomes an import-time
// "does not provide an export named ..." SyntaxError and the file collects
// zero tests. Spy-mode automocking wraps the real module instead: every
// current and future export stays bound, and only what must not really run is
// overridden below. `goto` is the one such binding — real navigation would
// steer the browser-mode page away mid-test. `beforeNavigate` is safe to keep
// real; it is a thin `onMount(() => callbacks.add(cb))` whose callbacks only
// fire on an actual client-router navigation, which never happens here.
//
// Spy mode rather than an `importOriginal` spread because that spread makes
// the mock a manually resolved module: the browser provider has to round-trip
// to this worker before it can answer the page's request for it, and when the
// round trip loses the race the run dies with "route.fulfill: The object has
// been collected" before a single test reports.
vi.mock("$app/navigation", { spy: true });

const mockGetFeedContent = vi.fn();
const mockGetArticleSourceURL = vi
	.fn()
	.mockResolvedValue({ url: "", title: "" });
vi.mock("$lib/api/client/articles", () => ({
	getFeedContentOnTheFlyClient: (...args: unknown[]) =>
		mockGetFeedContent(...args),
	getArticleSourceURLClient: (...args: unknown[]) =>
		mockGetArticleSourceURL(...args),
}));

const mockSummarizerReset = vi.fn();
let summarizerOverride: Partial<{
	summary: string | null;
	isSummarizing: boolean;
	summaryError: string | null;
	buttonState: "idle" | "loading" | "error" | "success";
}> = {};
vi.mock("$lib/hooks/useSummarize.svelte", () => ({
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

describe("Article page fetch button", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(goto).mockImplementation(async () => {});
		summarizerOverride = {};
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
			.element(testPage.getByRole("button", { name: /back to home/i }))
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
});

describe("Article page masthead", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		vi.mocked(goto).mockImplementation(async () => {});
		summarizerOverride = {};
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

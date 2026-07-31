import { render } from "vitest-browser-svelte";
import { page as testPage } from "vitest/browser";
import { describe, expect, it, vi, beforeEach } from "vitest";

vi.mock("$app/state", () => ({
	page: {
		params: { id: "test-123" },
		url: new URL(
			"http://localhost/articles/test-123?url=https%3A%2F%2Fexample.com%2Farticle&title=Test+Article",
		),
	},
}));

// The route reaches $app/navigation by two independent paths: +page.svelte
// imports `goto`, and useTrailOutcome — the trail dwell measurement — imports
// `beforeNavigate` to flush on leave. A factory that enumerates a subset
// replaces the *whole* module, so any binding it omits becomes an import-time
// "does not provide an export named ..." SyntaxError and the file collects
// zero tests. Enumerating one more name just moves the next rot one import
// away, so spread the real module instead: every current and future export
// stays bound, and only what must not really run is overridden. `goto` is the
// one such binding — real navigation would steer the browser-mode page away
// mid-test. `beforeNavigate` is safe to keep real; it is a thin
// `onMount(() => callbacks.add(cb))` whose callbacks only fire on an actual
// client-router navigation, which never happens here.
vi.mock("$app/navigation", async (importOriginal) => ({
	...(await importOriginal<typeof import("$app/navigation")>()),
	goto: vi.fn(),
}));

const mockGetFeedContent = vi.fn();
const mockGetArticleSourceURL = vi.fn().mockResolvedValue("");
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

import {
	createTtsPlaybackStore,
	TTS_PLAYBACK_KEY,
} from "$lib/stores/ttsPlayback.svelte";
import {
	createTtsPreferences,
	TTS_PREFERENCES_KEY,
} from "$lib/stores/ttsPreferences.svelte";
import Page from "./+page.svelte";

// +page.svelte reads the TTS playback and preferences stores out of context,
// and their only provider is (app)/+layout.svelte — a layout this spec does
// not mount. Rendered bare, getContext() yields undefined and the first
// `ttsPreferences.speed` read throws before any assertion runs. Reproduce the
// layout's own wiring with the real factories (both constructors are inert:
// field initialisers plus one localStorage read) instead of stubbing the
// stores, so the page is exercised against the objects it gets in production.
function renderPage() {
	return render(Page, {
		context: new Map<symbol, unknown>([
			[TTS_PLAYBACK_KEY, createTtsPlaybackStore()],
			[TTS_PREFERENCES_KEY, createTtsPreferences()],
		]),
	});
}

describe("Article page fetch button", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		summarizerOverride = {};
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
		summarizerOverride = {};
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

/**
 * FeedDetailModal content-state tests.
 *
 * The desktop modal is where ADR-000581's infinite auto-fetch `$effect` was
 * first found and closed, so every case here is written to fail loudly rather
 * than hang: each one pins the number of requests the modal sends on its own.
 */
import { Code, ConnectError } from "@connectrpc/connect";
import { page as testPage } from "@vitest/browser/context";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";
import type { RenderFeed } from "#lib/schema/feed.js";

const { mockSpeechPlay, mockSpeechStop, speechState } = vi.hoisted(() => ({
	mockSpeechPlay: vi.fn(),
	mockSpeechStop: vi.fn(),
	speechState: { value: "idle" },
}));

vi.mock("#lib/hooks/useSummarySpeech.svelte.js", () => ({
	createSummarySpeech: vi.fn(() => ({
		get state() {
			return speechState.value;
		},
		play: mockSpeechPlay,
		stop: mockSpeechStop,
	})),
}));

const mockGetFeedContent = vi.fn();
vi.mock("#lib/api/client/articles.js", () => ({
	getFeedContentOnTheFlyClient: (...args: unknown[]) =>
		mockGetFeedContent(...args),
}));

const mockTrackHomeActionClient = vi.fn();
vi.mock("#lib/api/client/knowledge_home.js", () => ({
	trackHomeActionClient: (...args: unknown[]) =>
		mockTrackHomeActionClient(...args),
}));

const mockGetItemBranches = vi.fn();
const mockResolveBranch = vi.fn();
vi.mock("#lib/connect/knowledge_trail.js", () => ({
	getItemBranches: (...args: unknown[]) => mockGetItemBranches(...args),
	resolveBranch: (...args: unknown[]) => mockResolveBranch(...args),
}));

vi.mock("#lib/connect/transport-client.js", () => ({
	createClientTransport: vi.fn(() => ({})),
}));

// Stubbed rather than exercised: the prefetcher is a module singleton whose
// cache would leak between cases, and its own behaviour has its own tests.
vi.mock("#lib/utils/articlePrefetcher.js", () => ({
	articlePrefetcher: {
		getCachedContent: () => null,
		getCachedArticleId: () => null,
		triggerPrefetch: vi.fn(),
	},
}));

vi.mock("#lib/connect/index.js", () => ({
	createClientTransport: vi.fn(() => ({})),
	streamSummarizeWithAbortAdapter: vi.fn(
		(
			_transport: unknown,
			_options: unknown,
			updateState?: (text: string) => void,
			_rendererOptions?: unknown,
			onComplete?: (result: unknown) => void,
			_onError?: (error: Error) => void,
		) => {
			updateState?.("This is a test summary.");
			if (onComplete) {
				onComplete({
					hasReceivedData: true,
					articleId: "article-123",
					chunkCount: 1,
					totalLength: 23,
					summary: "This is a test summary.",
					wasCached: false,
				});
			}
			return new AbortController();
		},
	),
}));

const mockRecordReadingEvents = vi.fn();
vi.mock("#lib/connect/reading.js", async (importOriginal) => {
	const actual =
		await importOriginal<typeof import("#lib/connect/reading.js")>();
	return {
		...actual,
		recordReadingEvents: (...args: unknown[]) =>
			mockRecordReadingEvents(...args),
		createKeepaliveTransport: vi.fn(() => ({})),
	};
});

import { ReadingSurface } from "#lib/connect/reading.js";
import { flushReadingEvents } from "#lib/reading/instrumentation.js";
import FeedDetailModal from "./FeedDetailModal.svelte";

const FEED: RenderFeed = {
	id: "feed-1",
	title: "Test Article Title",
	description: "The RSS description, which must survive every failure.",
	link: "https://example.com/test-article",
	published: "2026-08-01T10:00:00Z",
	created_at: "2026-08-01T09:00:00Z",
	author: "Test Author",
	publishedAtFormatted: "Aug 1, 2026",
	mergedTagsLabel: "Test / Svelte",
	normalizedUrl: "https://example.com/test-article",
	excerpt: "An excerpt.",
};

const connectError = (
	code: Code,
	headers: Record<string, string> = {},
): ConnectError =>
	new ConnectError(
		"Service temporarily unavailable due to circuit breaker",
		code,
		headers,
	);

function renderModal(readingSurface: ReadingSurface = ReadingSurface.GALLERY) {
	return render(FeedDetailModal, {
		props: { open: true, feed: FEED, onOpenChange: () => {}, readingSurface },
	});
}

describe("FeedDetailModal content states", () => {
	beforeEach(async () => {
		mockRecordReadingEvents.mockResolvedValue({ accepted: 0, duplicates: 0 });
		await flushReadingEvents();
		vi.clearAllMocks();
		mockRecordReadingEvents.mockResolvedValue({ accepted: 1, duplicates: 0 });
		mockSpeechPlay.mockReset();
		mockSpeechStop.mockReset();
		speechState.value = "idle";
	});

	it("says what it is doing while the body is in flight", async () => {
		mockGetFeedContent.mockReturnValue(new Promise(() => {}));

		renderModal();

		await expect
			.element(testPage.getByTestId("article-content-pending"))
			.toHaveTextContent("Fetching the full article");
		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.not.toBeInTheDocument();
		// The RSS body is on screen underneath the wait.
		await expect
			.element(testPage.getByTestId("article-fallback-summary"))
			.toHaveTextContent("The RSS description");
	});

	it("offers both remedies, and never the upstream prose, when it is terminal", async () => {
		mockGetFeedContent.mockRejectedValue(connectError(Code.Unavailable));

		renderModal();

		const failed = testPage.getByTestId("article-content-failed");
		await expect
			.element(failed)
			.toHaveTextContent(
				"Source content is temporarily unavailable. Please try again shortly.",
			);
		await expect.element(failed).not.toHaveTextContent("circuit breaker");
		await expect
			.element(testPage.getByTestId("retry-content"))
			.toBeInTheDocument();
		await expect
			.element(testPage.getByTestId("read-original-link"))
			.toHaveAttribute("href", FEED.link);
		await expect
			.element(testPage.getByTestId("article-fallback-summary"))
			.toBeInTheDocument();
	});

	it("states the shared wording for an empty body and asks exactly once", async () => {
		// ADR-000581's loop lived exactly here.
		mockGetFeedContent.mockResolvedValue({ content: "", article_id: "" });

		renderModal();

		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.toHaveTextContent("Source content unavailable.");
		await new Promise((r) => setTimeout(r, 600));
		expect(mockGetFeedContent).toHaveBeenCalledTimes(1);
	});

	it("does NOT auto-retry Code.Unavailable — ADR-000959 rejected exactly that", async () => {
		mockGetFeedContent.mockRejectedValue(
			connectError(Code.Unavailable, { "Retry-After": "0" }),
		);

		renderModal();

		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.toBeInTheDocument();
		await new Promise((r) => setTimeout(r, 900));
		expect(mockGetFeedContent).toHaveBeenCalledTimes(1);
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

		renderModal();

		await expect
			.element(testPage.getByTestId("article-content-pending"))
			.toHaveTextContent("Retrying");
		await expect
			.element(testPage.getByText("Arrived on the second attempt."))
			.toBeInTheDocument();
		expect(mockGetFeedContent).toHaveBeenCalledTimes(2);
	});

	it("does NOT retry a ResourceExhausted stamped as the publisher's own 429", async () => {
		mockGetFeedContent.mockRejectedValue(
			connectError(Code.ResourceExhausted, {
				"Retry-After": "0",
				"X-Alt-Failure-Scope": "host",
			}),
		);

		renderModal();

		await expect
			.element(testPage.getByTestId("article-content-failed"))
			.toBeInTheDocument();
		await new Promise((r) => setTimeout(r, 900));
		expect(mockGetFeedContent).toHaveBeenCalledTimes(1);
	});

	describe("AI summary speech button", () => {
		it("does not render the speech button while chunks are still typing before completion even though text is visible", async () => {
			mockGetFeedContent.mockResolvedValue({
				content: "<p>Article body</p>",
				article_id: "a1",
			});
			const { streamSummarizeWithAbortAdapter } = await import(
				"#lib/connect/index.js"
			);
			vi.mocked(streamSummarizeWithAbortAdapter).mockImplementationOnce(
				(
					_transport: unknown,
					_options: unknown,
					updateState?: (chunk: string) => void,
					rendererOptions?: {
						onChunk?: (
							count: number,
							size: number,
							decLen: number,
							totLen: number,
							preview: string,
						) => void;
					},
				) => {
					rendererOptions?.onChunk?.(1, 25, 25, 25, "Typing partial");
					updateState?.("Typing partial summary...");
					return new AbortController();
				},
			);

			renderModal();

			const summarizeBtn = testPage.getByRole("button", { name: /summarize/i });
			await expect.element(summarizeBtn).toBeEnabled();
			await summarizeBtn.click();

			await expect
				.element(testPage.getByText("Typing partial summary..."))
				.toBeInTheDocument();
			await expect
				.element(testPage.getByTestId("summary-speech-button"))
				.not.toBeInTheDocument();
		});

		it("renders speech button with 'Play summary' once summary stream completes via real update path", async () => {
			mockGetFeedContent.mockResolvedValue({
				content: "<p>Article body</p>",
				article_id: "a1",
			});

			renderModal();

			const summarizeBtn = testPage.getByRole("button", { name: /summarize/i });
			await expect.element(summarizeBtn).toBeEnabled();
			await summarizeBtn.click();

			const speechBtn = testPage.getByTestId("summary-speech-button");
			await expect.element(speechBtn).toBeInTheDocument();
			await expect.element(speechBtn).toHaveTextContent("Play summary");
		});

		it("clicking plays the full summary", async () => {
			mockGetFeedContent.mockResolvedValue({
				content: "<p>Article body</p>",
				article_id: "a1",
			});

			renderModal();

			const summarizeBtn = testPage.getByRole("button", { name: /summarize/i });
			await expect.element(summarizeBtn).toBeEnabled();
			await summarizeBtn.click();

			const speechBtn = testPage.getByTestId("summary-speech-button");
			await expect.element(speechBtn).toBeInTheDocument();

			await speechBtn.click();

			expect(mockSpeechPlay).toHaveBeenCalledWith("This is a test summary.");
		});

		it("speaks the full received summary text even when displayed text is still typing (cached single-chunk)", async () => {
			mockGetFeedContent.mockResolvedValue({
				content: "<p>Article body</p>",
				article_id: "a1",
			});
			const fullCachedText =
				"This is the full comprehensive cached summary that arrived in a single chunk.";
			const { streamSummarizeWithAbortAdapter } = await import(
				"#lib/connect/index.js"
			);
			vi.mocked(streamSummarizeWithAbortAdapter).mockImplementationOnce(
				(_transport, _options, updateState, _rendererOptions, onComplete) => {
					updateState?.("T");
					onComplete?.({
						hasReceivedData: true,
						articleId: "art-1",
						chunkCount: 1,
						totalLength: fullCachedText.length,
						summary: fullCachedText,
						wasCached: true,
					});
					return new AbortController();
				},
			);

			renderModal();

			const summarizeBtn = testPage.getByRole("button", { name: /summarize/i });
			await expect.element(summarizeBtn).toBeEnabled();
			await summarizeBtn.click();

			const speechBtn = testPage.getByTestId("summary-speech-button");
			await expect.element(speechBtn).toBeInTheDocument();

			await speechBtn.click();

			expect(mockSpeechPlay).toHaveBeenCalledWith(fullCachedText);
		});

		it("stops speech when the modal closes", async () => {
			mockGetFeedContent.mockResolvedValue({
				content: "<p>Article body</p>",
				article_id: "a1",
			});

			const { unmount } = renderModal();

			const summarizeBtn = testPage.getByRole("button", { name: /summarize/i });
			await expect.element(summarizeBtn).toBeEnabled();
			await summarizeBtn.click();

			const speechBtn = testPage.getByTestId("summary-speech-button");
			await expect.element(speechBtn).toBeInTheDocument();

			await speechBtn.click();
			expect(mockSpeechPlay).toHaveBeenCalledWith("This is a test summary.");

			unmount();

			expect(mockSpeechStop).toHaveBeenCalled();
		});
	});

	describe("open tracking and Home reader links", () => {
		it("records open once by default when article content loads", async () => {
			mockGetFeedContent.mockResolvedValue({
				content: "<p>Article body</p>",
				article_id: "art-default-open",
			});

			renderModal();

			await expect
				.element(testPage.getByText("Article body"))
				.toBeInTheDocument();

			expect(mockTrackHomeActionClient).toHaveBeenCalledTimes(1);
			expect(mockTrackHomeActionClient).toHaveBeenCalledWith(
				"open",
				"article:art-default-open",
				JSON.stringify({ source: "feed_detail_modal" }),
			);
		});

		it("does NOT record open when trackOpen is false", async () => {
			mockGetFeedContent.mockResolvedValue({
				content: "<p>Article body</p>",
				article_id: "art-home-open",
			});

			render(FeedDetailModal, {
				props: {
					open: true,
					feed: { ...FEED, articleId: "art-home-open" },
					onOpenChange: () => {},
					readingSurface: ReadingSurface.OTHER,
					trackOpen: false,
				},
			});

			await expect
				.element(testPage.getByText("Article body"))
				.toBeInTheDocument();

			expect(mockTrackHomeActionClient).not.toHaveBeenCalled();
		});

		it("does NOT render Home reader links by default", async () => {
			mockGetFeedContent.mockResolvedValue({
				content: "<p>Article body</p>",
				article_id: "art-default-links",
			});
			mockGetItemBranches.mockResolvedValue([]);

			renderModal();

			await expect
				.element(testPage.getByText("Article body"))
				.toBeInTheDocument();

			await expect
				.element(testPage.getByRole("link", { name: "Open article page" }))
				.not.toBeInTheDocument();
			expect(mockGetItemBranches).not.toHaveBeenCalled();
		});

		it("renders ArticleEndBranches and Open article page link when showHomeReaderLinks is true, and remounts on article change", async () => {
			mockGetFeedContent.mockResolvedValue({
				content: "<p>Article body</p>",
				article_id: "art-home-1",
			});
			mockGetItemBranches.mockResolvedValue([
				{
					branchKey: "cluster:u:article:next-1",
					anchorItemKey: "article:art-home-1",
					relationKind: "cluster",
					why: "Joins a topic you follow.",
					evidenceRefs: [{ refId: "rust", label: "rust", kind: "tag" }],
					confidence: "plausible",
					targetItemKey: "article:next-1",
					targetTitle: "Next Branch Article",
				},
			]);

			const { rerender } = render(FeedDetailModal, {
				props: {
					open: true,
					feed: { ...FEED, articleId: "art-home-1" },
					onOpenChange: () => {},
					readingSurface: ReadingSurface.OTHER,
					trackOpen: false,
					showHomeReaderLinks: true,
				},
			});

			await expect
				.element(testPage.getByText("Article body"))
				.toBeInTheDocument();

			const openArticleLink = testPage.getByRole("link", {
				name: "Open article page",
			});
			await expect.element(openArticleLink).toBeInTheDocument();
			await expect
				.element(openArticleLink)
				.toHaveAttribute("href", "/articles/art-home-1");

			await expect
				.element(testPage.getByTestId("article-end-branch"))
				.toBeInTheDocument();
			expect(mockGetItemBranches).toHaveBeenCalledTimes(1);
			expect(mockGetItemBranches).toHaveBeenCalledWith(
				expect.anything(),
				"article:art-home-1",
				2,
			);

			mockGetFeedContent.mockResolvedValue({
				content: "<p>Second article body</p>",
				article_id: "art/home 2?x=1",
			});
			mockGetItemBranches.mockResolvedValue([]);
			await rerender({
				open: true,
				feed: {
					...FEED,
					id: "feed-2",
					articleId: "art/home 2?x=1",
					link: "https://example.com/test-article-2",
					normalizedUrl: "https://example.com/test-article-2",
				},
				onOpenChange: () => {},
				readingSurface: ReadingSurface.OTHER,
				trackOpen: false,
				showHomeReaderLinks: true,
			});

			await expect
				.element(testPage.getByRole("link", { name: "Open article page" }))
				.toHaveAttribute("href", "/articles/art%2Fhome%202%3Fx%3D1");
			expect(mockGetItemBranches).toHaveBeenCalledTimes(2);
			expect(mockGetItemBranches).toHaveBeenLastCalledWith(
				expect.anything(),
				"article:art/home 2?x=1",
				2,
			);
		});
	});

	describe("reading session recording", () => {
		it("starts a session when body and article id are present, records reached_end, and flushes session_ended on close", async () => {
			const longBody = Array.from(
				{ length: 24 },
				(_, i) =>
					`<p>Paragraph ${i + 1} with substantial prose for scrolling inside the modal body container.</p>`,
			).join("");
			mockGetFeedContent.mockResolvedValue({
				content: longBody,
				article_id: "art-read-1",
			});

			const { rerender } = renderModal();

			await expect
				.element(testPage.getByText(/Paragraph 1 with substantial prose/))
				.toBeInTheDocument();

			const modalBody = document.querySelector(".modal-body") as HTMLElement;
			expect(modalBody).not.toBeNull();
			modalBody.scrollTop = modalBody.scrollHeight;
			modalBody.dispatchEvent(new Event("scroll"));
			await new Promise((r) => setTimeout(r, 50));

			expect(mockRecordReadingEvents).not.toHaveBeenCalled();

			await rerender({
				open: false,
				feed: FEED,
				onOpenChange: () => {},
				readingSurface: ReadingSurface.GALLERY,
			});

			await vi.waitFor(() =>
				expect(mockRecordReadingEvents).toHaveBeenCalledTimes(1),
			);
			const events = mockRecordReadingEvents.mock.calls[0]?.[1] ?? [];
			const cases = events.map(
				(e: { event: { case: string } }) => e.event.case,
			);
			expect(cases).toContain("reachedEnd");
			expect(cases).toContain("sessionEnded");
			const sessionEnded = events.find(
				(e: { event: { case: string } }) => e.event.case === "sessionEnded",
			)?.event.value;
			expect(sessionEnded).toMatchObject({
				articleId: "art-read-1",
				surface: ReadingSurface.GALLERY,
				reachedEnd: true,
			});
			expect(sessionEnded.bodyLengthChars).toBeGreaterThan(0);
		});

		it("ends the current session when navigating to another feed inside the modal", async () => {
			mockGetFeedContent
				.mockResolvedValueOnce({
					content: "<p>First modal article body.</p>",
					article_id: "art-nav-1",
				})
				.mockResolvedValueOnce({
					content: "<p>Second modal article body.</p>",
					article_id: "art-nav-2",
				});

			const { rerender, unmount } = renderModal();

			await expect
				.element(testPage.getByText("First modal article body."))
				.toBeInTheDocument();

			await rerender({
				open: true,
				feed: {
					...FEED,
					id: "feed-2",
					articleId: "art-nav-2",
					link: "https://example.com/second-article",
					normalizedUrl: "https://example.com/second-article",
				},
				onOpenChange: () => {},
				readingSurface: ReadingSurface.GALLERY,
			});

			await vi.waitFor(() =>
				expect(mockRecordReadingEvents).toHaveBeenCalledTimes(1),
			);
			const firstEvents = mockRecordReadingEvents.mock.calls[0]?.[1] ?? [];
			const firstSessionEnded = firstEvents.find(
				(e: { event: { case: string } }) => e.event.case === "sessionEnded",
			)?.event.value;
			expect(firstSessionEnded?.articleId).toBe("art-nav-1");

			await expect
				.element(testPage.getByText("Second modal article body."))
				.toBeInTheDocument();

			unmount();
			await vi.waitFor(() =>
				expect(mockRecordReadingEvents).toHaveBeenCalledTimes(2),
			);
			const secondEvents = mockRecordReadingEvents.mock.calls[1]?.[1] ?? [];
			const secondSessionEnded = secondEvents.find(
				(e: { event: { case: string } }) => e.event.case === "sessionEnded",
			)?.event.value;
			expect(secondSessionEnded?.articleId).toBe("art-nav-2");
		});

		it("records no session when content fetch fails or when article id remains unknown", async () => {
			mockGetFeedContent.mockRejectedValueOnce(connectError(Code.Unavailable));

			const first = renderModal();
			await expect
				.element(testPage.getByTestId("article-content-failed"))
				.toBeInTheDocument();
			first.unmount();

			mockGetFeedContent.mockResolvedValueOnce({
				content: "<p>Body with no stored article id.</p>",
				article_id: "",
			});

			const second = renderModal();
			await expect
				.element(testPage.getByText("Body with no stored article id."))
				.toBeInTheDocument();
			second.unmount();

			await flushReadingEvents();
			expect(mockRecordReadingEvents).not.toHaveBeenCalled();
		});

		it("records the supplied ReadingSurface.OTHER prop on reachedEnd and sessionEnded", async () => {
			const longBody = Array.from(
				{ length: 24 },
				(_, i) =>
					`<p>Paragraph ${i + 1} with substantial prose for other surface attribution testing.</p>`,
			).join("");
			mockGetFeedContent.mockResolvedValueOnce({
				content: longBody,
				article_id: "art-other-surface",
			});

			const { rerender } = renderModal(ReadingSurface.OTHER);

			await expect
				.element(testPage.getByText(/Paragraph 1 with substantial prose/))
				.toBeInTheDocument();

			const modalBody = document.querySelector(".modal-body") as HTMLElement;
			modalBody.scrollTop = modalBody.scrollHeight;
			modalBody.dispatchEvent(new Event("scroll"));
			await new Promise((r) => setTimeout(r, 50));

			await rerender({
				open: false,
				feed: FEED,
				onOpenChange: () => {},
				readingSurface: ReadingSurface.OTHER,
			});

			await vi.waitFor(() =>
				expect(mockRecordReadingEvents).toHaveBeenCalledTimes(1),
			);
			const events = mockRecordReadingEvents.mock.calls[0]?.[1] ?? [];
			const reachedEnd = events.find(
				(e: { event: { case: string } }) => e.event.case === "reachedEnd",
			)?.event.value;
			const sessionEnded = events.find(
				(e: { event: { case: string } }) => e.event.case === "sessionEnded",
			)?.event.value;
			expect(reachedEnd).toMatchObject({
				articleId: "art-other-surface",
				surface: ReadingSurface.OTHER,
			});
			expect(sessionEnded).toMatchObject({
				articleId: "art-other-surface",
				surface: ReadingSurface.OTHER,
			});
		});

		it("does not fragment the session when the parent replaces feed with a new object for the same article", async () => {
			const longBody = Array.from(
				{ length: 24 },
				(_, i) =>
					`<p>Paragraph ${i + 1} with substantial prose to guard against object-identity restarts.</p>`,
			).join("");
			mockGetFeedContent.mockResolvedValueOnce({
				content: longBody,
				article_id: "art-stable-1",
			});

			const { rerender } = renderModal(ReadingSurface.GALLERY);

			await expect
				.element(testPage.getByText(/Paragraph 1 with substantial prose/))
				.toBeInTheDocument();

			// Parent replaces the feed object with identical values (e.g. list refresh after open).
			await rerender({
				open: true,
				feed: { ...FEED },
				onOpenChange: () => {},
				readingSurface: ReadingSurface.GALLERY,
			});

			const modalBody = document.querySelector(".modal-body") as HTMLElement;
			modalBody.scrollTop = modalBody.scrollHeight;
			modalBody.dispatchEvent(new Event("scroll"));
			await new Promise((r) => setTimeout(r, 50));

			expect(mockRecordReadingEvents).not.toHaveBeenCalled();

			await rerender({
				open: false,
				feed: { ...FEED },
				onOpenChange: () => {},
				readingSurface: ReadingSurface.GALLERY,
			});

			await vi.waitFor(() =>
				expect(mockRecordReadingEvents).toHaveBeenCalledTimes(1),
			);
			const events = mockRecordReadingEvents.mock.calls[0]?.[1] ?? [];
			const sessionEndeds = events.filter(
				(e: { event: { case: string } }) => e.event.case === "sessionEnded",
			);
			const reachedEnds = events.filter(
				(e: { event: { case: string } }) => e.event.case === "reachedEnd",
			);
			expect(sessionEndeds).toHaveLength(1);
			expect(reachedEnds).toHaveLength(1);
		});

		it("resets modal-body scrollTop to 0 when navigating to a different article", async () => {
			await testPage.viewport(393, 600);
			const longFirstBody = Array.from(
				{ length: 45 },
				(_, i) =>
					`<p>First article paragraph ${i + 1} with substantial prose for scrolling.</p>`,
			).join("");
			const longSecondBody = Array.from(
				{ length: 45 },
				(_, i) =>
					`<p>Second article paragraph ${i + 1} with substantial prose for scrolling.</p>`,
			).join("");
			mockGetFeedContent
				.mockResolvedValueOnce({
					content: longFirstBody,
					article_id: "art-scroll-1",
				})
				.mockResolvedValueOnce({
					content: longSecondBody,
					article_id: "art-scroll-2",
				});

			const { rerender, unmount } = renderModal(ReadingSurface.GALLERY);

			await expect
				.element(testPage.getByText(/First article paragraph 1 /))
				.toBeInTheDocument();

			const modalBody = document.querySelector(".modal-body") as HTMLElement;
			modalBody.style.height = "200px";
			modalBody.style.overflowY = "auto";
			modalBody.scrollTop = modalBody.scrollHeight;
			modalBody.dispatchEvent(new Event("scroll"));
			expect(modalBody.scrollTop).toBeGreaterThan(0);

			await rerender({
				open: true,
				feed: {
					...FEED,
					id: "feed-scroll-2",
					articleId: "art-scroll-2",
					link: "https://example.com/scroll-2",
					normalizedUrl: "https://example.com/scroll-2",
				},
				onOpenChange: () => {},
				readingSurface: ReadingSurface.GALLERY,
			});

			await expect
				.element(testPage.getByText(/Second article paragraph 1 /))
				.toBeInTheDocument();
			expect(modalBody.scrollTop).toBe(0);

			unmount();
			await vi.waitFor(() =>
				expect(mockRecordReadingEvents).toHaveBeenCalledTimes(2),
			);
			const secondEvents = mockRecordReadingEvents.mock.calls[1]?.[1] ?? [];
			const secondSessionEnded = secondEvents.find(
				(e: { event: { case: string } }) => e.event.case === "sessionEnded",
			)?.event.value;
			expect(secondSessionEnded).toMatchObject({
				articleId: "art-scroll-2",
				reachedEnd: false,
			});
		});
	});
});

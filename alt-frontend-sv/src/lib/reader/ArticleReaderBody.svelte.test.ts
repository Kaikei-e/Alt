import { Code, ConnectError } from "@connectrpc/connect";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { page as testPage } from "vitest/browser";
import { render } from "vitest-browser-svelte";

vi.mock("#lib/connect/knowledge_trail.js", { spy: true });
vi.mock("#lib/connect/reading.js", { spy: true });
vi.mock("#lib/connect/transport-client.js", { spy: true });

import { getItemBranches } from "#lib/connect/knowledge_trail.js";
import { recordReadingEvents } from "#lib/connect/reading.js";
import { createClientTransport } from "#lib/connect/transport-client.js";
import {
	flushReadingEvents,
	ReadingSurface,
} from "#lib/reading/instrumentation.js";

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

const mockSpeechPlay = vi.fn();
const mockSpeechStop = vi.fn();
let speechStateOverride = $state<
	"idle" | "loading" | "playing" | "error" | "unavailable"
>("idle");
vi.mock("#lib/hooks/useSummarySpeech.svelte.js", () => ({
	createSummarySpeech: () => ({
		get state() {
			return speechStateOverride;
		},
		play: mockSpeechPlay,
		stop: mockSpeechStop,
	}),
}));

const mockSummarize = vi.fn();
const mockSummarizerReset = vi.fn();
const mockSummarizerAbort = vi.fn();
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
		summarize: mockSummarize,
		abort: mockSummarizerAbort,
		reset: mockSummarizerReset,
		...summarizerOverride,
	}),
}));

import ArticleReaderBody from "./ArticleReaderBody.svelte";
import { createArticleReader } from "./createArticleReader.svelte.js";

describe("ArticleReaderBody + createArticleReader", () => {
	beforeEach(async () => {
		vi.clearAllMocks();
		vi.mocked(recordReadingEvents).mockResolvedValue({
			accepted: 0,
			duplicates: 0,
			readArticleIds: [],
		});
		await flushReadingEvents();
		vi.mocked(recordReadingEvents).mockClear();
		vi.mocked(recordReadingEvents).mockResolvedValue({
			accepted: 1,
			duplicates: 0,
			readArticleIds: [],
		});
		vi.mocked(getItemBranches).mockResolvedValue([]);
		vi.mocked(createClientTransport).mockReturnValue(
			{} as ReturnType<typeof createClientTransport>,
		);
		summarizerOverride = {};
		speechStateOverride = "idle";
	});

	it("renders fetched content, retries unstamped ResourceExhausted once, and ends reading session on unmount", async () => {
		mockGetFeedContent
			.mockRejectedValueOnce(
				new ConnectError("Rate limited", Code.ResourceExhausted, {
					"Retry-After": "0",
				}),
			)
			.mockResolvedValueOnce({
				content: "<p>Unified reader body.</p>",
				article_id: "art-1",
			});

		let reader!: ReturnType<typeof createArticleReader>;
		const cleanup = $effect.root(() => {
			reader = createArticleReader({
				getArticleId: () => "art-1",
				getUrl: () => "https://example.com/unified",
				getTitle: () => "Unified Title",
				getSurface: () => ReadingSurface.ARTICLE_PAGE,
			});
		});

		const { unmount } = render(ArticleReaderBody, {
			props: { reader },
		});

		await expect
			.element(testPage.getByText("Unified reader body."))
			.toBeInTheDocument();
		expect(mockGetFeedContent).toHaveBeenCalledTimes(2);

		unmount();
		cleanup();

		await vi.waitFor(() =>
			expect(recordReadingEvents).toHaveBeenCalledTimes(1),
		);
		const events = vi.mocked(recordReadingEvents).mock.calls[0]?.[1] ?? [];
		expect(events.map((e) => e.event?.case)).toContain("sessionEnded");
	});

	it("shows read-aloud control when summary completes and plays/stops speech", async () => {
		summarizerOverride = {
			summary: "Concise article summary.",
			isSummarizing: false,
			summaryError: null,
			buttonState: "success",
		};
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Unified reader body.</p>",
			article_id: "art-2",
		});

		let reader!: ReturnType<typeof createArticleReader>;
		const cleanup = $effect.root(() => {
			reader = createArticleReader({
				getArticleId: () => "art-2",
				getUrl: () => "https://example.com/speech",
				getTitle: () => "Speech Article",
				getSurface: () => ReadingSurface.GALLERY,
			});
		});

		const { unmount } = render(ArticleReaderBody, {
			props: { reader },
		});

		const speechBtn = testPage.getByTestId("summary-speech-button");
		await expect.element(speechBtn).toBeInTheDocument();
		await speechBtn.click();
		expect(mockSpeechPlay).toHaveBeenCalledWith("Concise article summary.");

		speechStateOverride = "playing";
		await expect.element(speechBtn).toHaveTextContent("Stop");
		await speechBtn.click();
		expect(mockSpeechStop).toHaveBeenCalled();

		unmount();
		cleanup();
		expect(mockSpeechStop).toHaveBeenCalled();
	});

	it("reserves article-end connections slot without rendering an empty frame when omitted", async () => {
		mockGetFeedContent.mockResolvedValueOnce({
			content: "<p>Body for connections test.</p>",
			article_id: "art-3",
		});

		let reader!: ReturnType<typeof createArticleReader>;
		const cleanup = $effect.root(() => {
			reader = createArticleReader({
				getArticleId: () => "art-3",
				getUrl: () => "https://example.com/conn",
				getTitle: () => "Conn Article",
				getSurface: () => ReadingSurface.ARTICLE_PAGE,
			});
		});

		const { container, unmount } = render(ArticleReaderBody, {
			props: { reader },
		});

		await expect
			.element(testPage.getByText("Body for connections test."))
			.toBeInTheDocument();

		expect(
			container.querySelector('[data-testid="article-end-connections"]'),
		).toBeNull();
		expect(
			container.querySelector('[data-testid="article-end-branches"]'),
		).toBeNull();

		unmount();
		cleanup();
	});
});

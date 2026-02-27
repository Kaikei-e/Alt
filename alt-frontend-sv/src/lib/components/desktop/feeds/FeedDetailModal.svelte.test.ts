import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import FeedDetailModal from "./FeedDetailModal.svelte";
import type { RenderFeed } from "$lib/schema/feed";

// Mock TTS playback hook
const mockPlay = vi.fn();
const mockStop = vi.fn();
let mockIsPlaying = false;
let mockIsLoading = false;
let mockError: string | null = null;

vi.mock("$lib/hooks/useTtsPlayback.svelte", () => ({
	useTtsPlayback: () => ({
		get isPlaying() {
			return mockIsPlaying;
		},
		get isLoading() {
			return mockIsLoading;
		},
		get error() {
			return mockError;
		},
		get state() {
			return mockIsPlaying ? "playing" : mockIsLoading ? "loading" : "idle";
		},
		play: mockPlay,
		stop: mockStop,
	}),
}));

// Mock connect module
vi.mock("$lib/connect", () => ({
	createClientTransport: vi.fn(() => ({})),
	streamSummarizeWithAbortAdapter: vi.fn(),
}));

// Mock API client
vi.mock("$lib/api/client/articles", () => ({
	getFeedContentOnTheFlyClient: vi.fn().mockResolvedValue({
		content: "",
		article_id: "",
	}),
}));

// Mock article prefetcher
vi.mock("$lib/utils/articlePrefetcher", () => ({
	articlePrefetcher: {
		triggerPrefetch: vi.fn(),
		getCachedContent: vi.fn(() => null),
		getCachedArticleId: vi.fn(() => null),
	},
}));

const baseFeed: RenderFeed = {
	id: "1",
	title: "Test Article",
	link: "https://example.com/article",
	normalizedUrl: "https://example.com/article",
	excerpt: "Test excerpt",
	author: "Author",
	publishedAtFormatted: "2026-01-01",
	mergedTagsLabel: "",
	feedName: "Test Feed",
	isRead: false,
	isBookmarked: false,
};

describe("FeedDetailModal TTS integration", () => {
	beforeEach(() => {
		vi.clearAllMocks();
		mockIsPlaying = false;
		mockIsLoading = false;
		mockError = null;
	});

	it("does not show TTS button when summary is null", async () => {
		render(FeedDetailModal, {
			props: {
				open: true,
				feed: baseFeed,
				onOpenChange: vi.fn(),
			},
		});

		// The modal should render but no TTS button (no summary)
		const ttsButton = page.getByRole("button", { name: /read aloud/i });
		await expect.element(ttsButton).not.toBeInTheDocument();
	});
});

import { beforeEach, describe, expect, it, vi } from "vitest";
import { page as testPage } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type {
	KnowledgeHomeItemData,
	LensData,
	RecallCandidateData,
} from "#lib/connect/knowledge_home.js";
import { goto } from "$app/navigation";

const {
	mockHomeTrackAction,
	mockHomeDismissItem,
	mockHomeFetchData,
	mockHomeLoadMore,
	mockHomeTrackSeen,
	mockRecallSetCandidates,
	mockRecallSnooze,
	mockRecallDismiss,
	mockLensFetchLenses,
	mockLensCreate,
	mockLensRemove,
	mockLensSelect,
	mockLensReset,
	mockToastPush,
	mockToastRemove,
	mockGetFeedContent,
	mockTrackHomeActionClient,
	mockGetItemBranches,
	mockListSubscriptions,
} = vi.hoisted(() => ({
	mockHomeTrackAction: vi.fn(),
	mockHomeDismissItem: vi.fn(),
	mockHomeFetchData: vi.fn().mockResolvedValue(undefined),
	mockHomeLoadMore: vi.fn(),
	mockHomeTrackSeen: vi.fn(),
	mockRecallSetCandidates: vi.fn(),
	mockRecallSnooze: vi.fn(),
	mockRecallDismiss: vi.fn(),
	mockLensFetchLenses: vi.fn().mockResolvedValue(undefined),
	mockLensCreate: vi.fn(),
	mockLensRemove: vi.fn(),
	mockLensSelect: vi.fn().mockResolvedValue(undefined),
	mockLensReset: vi.fn(),
	mockToastPush: vi.fn(),
	mockToastRemove: vi.fn(),
	mockGetFeedContent: vi.fn(),
	mockTrackHomeActionClient: vi.fn().mockResolvedValue(undefined),
	mockGetItemBranches: vi.fn().mockResolvedValue([]),
	mockListSubscriptions: vi.fn().mockResolvedValue([]),
}));

let mockItems: KnowledgeHomeItemData[] = [];
let mockRecallCandidates: RecallCandidateData[] = [];
let mockLenses: LensData[] = [];
let mockActiveLensId: string | null = null;

vi.mock("$app/env", () => ({
	browser: true,
}));

vi.mock("$app/state", () => ({
	page: {
		url: new URL("http://localhost/home"),
	},
}));

vi.mock("$app/navigation", () => ({
	goto: vi.fn().mockResolvedValue(undefined),
	beforeNavigate: vi.fn(),
}));

vi.mock("#lib/stores/viewport.svelte.js", () => ({
	isDesktop: () => true,
}));

vi.mock("#lib/hooks/useKnowledgeHome.svelte.js", () => ({
	useKnowledgeHome: () => ({
		get items() {
			return mockItems;
		},
		get recallCandidates() {
			return mockRecallCandidates;
		},
		digest: null,
		loading: false,
		hasMore: false,
		pageState: "ready",
		serviceQuality: "full",
		emptyReason: null,
		trackAction: mockHomeTrackAction,
		dismissItem: mockHomeDismissItem,
		fetchData: mockHomeFetchData,
		loadMore: mockHomeLoadMore,
		trackSeen: mockHomeTrackSeen,
	}),
}));

vi.mock("#lib/hooks/useRecallRail.svelte.js", () => ({
	useRecallRail: () => ({
		get candidates() {
			return mockRecallCandidates;
		},
		setCandidates: mockRecallSetCandidates,
		snooze: mockRecallSnooze,
		dismiss: mockRecallDismiss,
	}),
}));

vi.mock("#lib/hooks/useLens.svelte.js", () => ({
	useLens: () => ({
		get lenses() {
			return mockLenses;
		},
		get activeLensId() {
			return mockActiveLensId;
		},
		loading: false,
		error: null,
		fetchLenses: mockLensFetchLenses,
		create: mockLensCreate,
		remove: mockLensRemove,
		select: mockLensSelect,
		reset: mockLensReset,
	}),
}));

vi.mock("#lib/hooks/useStreamUpdates.svelte.js", () => ({
	useStreamUpdates: () => ({
		pendingCount: 0,
		isConnected: true,
		isFallback: false,
		applyUpdates: vi.fn(),
	}),
}));

vi.mock("#lib/stores/toast.svelte.js", () => ({
	useToastStore: () => ({
		items: [],
		push: mockToastPush,
		remove: mockToastRemove,
	}),
}));

vi.mock("#lib/api/client/articles.js", () => ({
	getFeedContentOnTheFlyClient: (...args: unknown[]) =>
		mockGetFeedContent(...args),
}));

vi.mock("#lib/api/client/knowledge_home.js", () => ({
	trackHomeActionClient: (...args: unknown[]) =>
		mockTrackHomeActionClient(...args),
}));

vi.mock("#lib/connect/knowledge_trail.js", () => ({
	getItemBranches: (...args: unknown[]) => mockGetItemBranches(...args),
	resolveBranch: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("#lib/connect/transport-client.js", () => ({
	createClientTransport: vi.fn(() => ({})),
}));

vi.mock("#lib/utils/articlePrefetcher.js", () => ({
	articlePrefetcher: {
		getCachedContent: () => null,
		getCachedArticleId: () => null,
		triggerPrefetch: vi.fn(),
	},
}));

vi.mock("#lib/hooks/useSummarySpeech.svelte.js", () => ({
	createSummarySpeech: vi.fn(() => ({
		state: "idle",
		play: vi.fn(),
		stop: vi.fn(),
	})),
}));

vi.mock("#lib/connect/index.js", () => ({
	createClientTransport: vi.fn(() => ({})),
	listSubscriptions: (...args: unknown[]) => mockListSubscriptions(...args),
	streamSummarizeWithAbortAdapter: vi.fn(() => new AbortController()),
	streamAugurChat: vi.fn(),
}));

import HomePage from "./+page.svelte";

function makeHomeItem(
	overrides: Partial<KnowledgeHomeItemData> = {},
): KnowledgeHomeItemData {
	return {
		itemKey: "article:art-1",
		itemType: "article",
		articleId: "art-1",
		title: "First Home Article",
		url: "https://example.com/art-1",
		publishedAt: "2026-10-14T10:00:00Z",
		summaryExcerpt: "Excerpt one.",
		summaryState: "ready",
		tags: ["rust"],
		why: [],
		score: 0.9,
		...overrides,
	};
}

function countAllRecordedOpens(): number {
	const homeOpens = mockHomeTrackAction.mock.calls.filter(
		(call) => call[0] === "open",
	).length;
	const modalOpens = mockTrackHomeActionClient.mock.calls.filter(
		(call) => call[0] === "open",
	).length;
	return homeOpens + modalOpens;
}

describe("Knowledge Home open tracking and lens deletion", () => {
	beforeEach(async () => {
		vi.clearAllMocks();
		await testPage.viewport(1024, 768);
		mockItems = [
			makeHomeItem({
				itemKey: "article:art-1",
				articleId: "art-1",
				title: "First Home Article",
				url: "https://example.com/art-1",
			}),
			makeHomeItem({
				itemKey: "article:art-2",
				articleId: "art-2",
				title: "Second Home Article",
				url: "https://example.com/art-2",
			}),
		];
		mockRecallCandidates = [
			{
				itemKey: "article:recall-1",
				recallScore: 0.8,
				reasons: [{ type: "not_revisited", description: "Not revisited" }],
				firstEligibleAt: "2026-10-14T09:00:00Z",
				nextSuggestAt: "2026-10-15T09:00:00Z",
				item: makeHomeItem({
					itemKey: "article:recall-1",
					articleId: "recall-1",
					title: "Recall Article With URL",
					url: "https://example.com/recall-1",
				}),
			},
		];
		mockLenses = [
			{
				lensId: "lens-1",
				name: "Rust Focus",
				description: "Rust items",
				createdAt: "2026-10-14T00:00:00Z",
				updatedAt: "2026-10-14T00:00:00Z",
				currentVersion: {
					versionId: "v1",
					queryText: "rust",
					tagIds: ["rust"],
					sourceIds: [],
					timeWindow: "7d",
					includeRecap: false,
					includePulse: false,
					sortMode: "relevance",
				},
			},
		];
		mockActiveLensId = null;
	});

	it("records open exactly once on card open, and once per reader next/previous step", async () => {
		mockGetFeedContent.mockImplementation((url: string) =>
			Promise.resolve({
				content: `<p>Full content for ${url}</p>`,
				article_id: url.endsWith("art-2") ? "art-2" : "art-1",
			}),
		);

		render(HomePage);

		await expect
			.element(testPage.getByText("First Home Article"))
			.toBeInTheDocument();

		await testPage
			.getByRole("button", { name: "First Home Article", exact: true })
			.click();

		await expect
			.element(testPage.getByText("Full content for https://example.com/art-1"))
			.toBeInTheDocument();

		expect(countAllRecordedOpens()).toBe(1);
		expect(mockHomeTrackAction).toHaveBeenCalledWith(
			"open",
			"article:art-1",
			undefined,
		);
		expect(mockTrackHomeActionClient).not.toHaveBeenCalled();

		// Reader next
		mockHomeTrackAction.mockClear();
		mockTrackHomeActionClient.mockClear();

		await testPage.getByRole("button", { name: "Next feed" }).click();
		await expect
			.element(testPage.getByText("Full content for https://example.com/art-2"))
			.toBeInTheDocument();

		expect(countAllRecordedOpens()).toBe(1);
		expect(mockHomeTrackAction).toHaveBeenCalledWith(
			"open",
			"article:art-2",
			undefined,
		);
		expect(mockTrackHomeActionClient).not.toHaveBeenCalled();

		// Reader previous
		mockHomeTrackAction.mockClear();
		mockTrackHomeActionClient.mockClear();

		await testPage.getByRole("button", { name: "Previous feed" }).click();
		await expect
			.element(testPage.getByText("Full content for https://example.com/art-1"))
			.toBeInTheDocument();

		expect(countAllRecordedOpens()).toBe(1);
		expect(mockHomeTrackAction).toHaveBeenCalledWith(
			"open",
			"article:art-1",
			undefined,
		);
		expect(mockTrackHomeActionClient).not.toHaveBeenCalled();
	});

	it("records open exactly once on recall open", async () => {
		mockGetFeedContent.mockResolvedValue({
			content: "<p>Recall full body</p>",
			article_id: "recall-1",
		});

		render(HomePage);

		const recallCard = testPage.getByText("Recall Article With URL");
		await expect.element(recallCard).toBeInTheDocument();
		await recallCard.click();

		await expect
			.element(testPage.getByText("Recall full body"))
			.toBeInTheDocument();

		expect(countAllRecordedOpens()).toBe(1);
		expect(mockHomeTrackAction).toHaveBeenCalledWith(
			"open",
			"article:recall-1",
		);
		expect(mockTrackHomeActionClient).not.toHaveBeenCalled();
	});

	it("does NOT record open on Home when card or recall item has no URL and navigates to /articles/<id>", async () => {
		mockItems = [
			makeHomeItem({
				itemKey: "article:no-url-1",
				articleId: "no-url-1",
				title: "No URL Card Article",
				url: undefined,
			}),
		];
		mockRecallCandidates = [
			{
				itemKey: "article:no-url-recall",
				recallScore: 0.7,
				reasons: [{ type: "not_revisited", description: "Not revisited" }],
				firstEligibleAt: "2026-10-14T09:00:00Z",
				nextSuggestAt: "2026-10-15T09:00:00Z",
				item: makeHomeItem({
					itemKey: "article:no-url-recall",
					articleId: "no-url-recall",
					title: "No URL Recall Article",
					url: undefined,
				}),
			},
		];

		render(HomePage);

		await expect
			.element(testPage.getByText("No URL Card Article"))
			.toBeInTheDocument();

		await testPage.getByRole("button", { name: "Open", exact: true }).click();

		expect(goto).toHaveBeenCalledWith("/articles/no-url-1");
		expect(countAllRecordedOpens()).toBe(0);

		vi.mocked(goto).mockClear();
		await testPage.getByText("No URL Recall Article").click();

		expect(goto).toHaveBeenCalledWith("/articles/no-url-recall");
		expect(countAllRecordedOpens()).toBe(0);
	});

	it("shows 'Lens deleted.' on success and 'Failed to delete lens.' on failure", async () => {
		mockLensRemove.mockResolvedValueOnce(false).mockResolvedValueOnce(true);

		render(HomePage);

		const deleteBtn = testPage.getByRole("button", {
			name: "Delete lens Rust Focus",
		});
		await expect.element(deleteBtn).toBeInTheDocument();

		// First attempt: fails
		await deleteBtn.click();
		await testPage.getByRole("button", { name: "Confirm" }).click();

		expect(mockLensRemove).toHaveBeenCalledWith("lens-1");
		expect(mockToastPush).toHaveBeenCalledWith(
			"Failed to delete lens.",
			"error",
		);
		expect(mockToastPush).not.toHaveBeenCalledWith(
			"Lens deleted.",
			expect.anything(),
		);

		// Second attempt: succeeds
		mockToastPush.mockClear();
		await deleteBtn.click();
		await testPage.getByRole("button", { name: "Confirm" }).click();

		expect(mockToastPush).toHaveBeenCalledWith("Lens deleted.", "success");
	});
});

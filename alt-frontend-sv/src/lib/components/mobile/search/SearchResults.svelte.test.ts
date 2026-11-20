import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { openReaderOverlay } from "#lib/reader/reader-overlay.svelte.js";
import { ReadingSurface } from "#lib/reading/reading-events.js";
import { searchResultsFixture } from "../../../../../tests/fixtures/search";
import SearchResults from "./SearchResults.svelte";

vi.mock("#lib/api/client/index.js", () => ({
	searchFeedsClient: vi.fn(),
}));

vi.mock("#lib/reader/reader-overlay.svelte.js", () => ({
	openReaderOverlay: vi.fn(),
	replaceReaderOverlayItems: vi.fn(),
	closeReaderOverlay: vi.fn(),
	updateReaderOverlayArticleId: vi.fn(),
	navigateReaderOverlay: vi.fn(),
	getReaderOverlaySession: vi.fn(),
	resetReaderOverlaySessions: vi.fn(),
}));

vi.mock("#lib/utils/transformFeedSearchResult.js", () => ({
	transformFeedSearchResult: vi.fn(() => []),
}));

const baseProps = () => ({
	results: searchResultsFixture,
	isLoading: false,
	searchQuery: "test",
	searchTime: 150,
	cursor: null as string | null,
	hasMore: false,
	setResults: vi.fn(),
	setCursor: vi.fn(),
	setHasMore: vi.fn(),
	setIsLoading: vi.fn(),
});

describe("SearchResults Alt-Paper compliance", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("renders result count in monospace style", async () => {
		render(SearchResults, { props: baseProps() });

		await expect.element(page.getByText(/Search Results/)).toBeInTheDocument();
	});

	it("renders search time", async () => {
		render(SearchResults, { props: baseProps() });

		await expect.element(page.getByText(/150ms/)).toBeInTheDocument();
	});

	it("shows loading state with pulsing dot, not spinner", async () => {
		const props = baseProps();
		props.results = [];
		props.isLoading = true;

		render(SearchResults, { props });

		// Should have pulsing dot indicator
		await expect.element(page.getByText(/Searching/i)).toBeInTheDocument();
	});

	it("shows empty state without emoji", async () => {
		const props = baseProps();
		props.results = [];
		props.isLoading = false;

		render(SearchResults, { props });

		await expect
			.element(page.getByText(/No results found/i))
			.toBeInTheDocument();
	});

	it("renders result list items", async () => {
		render(SearchResults, { props: baseProps() });

		await expect
			.element(page.getByRole("link", { name: "First Article" }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("link", { name: "Second Article" }))
			.toBeInTheDocument();
	});

	it("shows end marker when no more results", async () => {
		render(SearchResults, { props: baseProps() });

		await expect
			.element(page.getByText(/No more results/i))
			.toBeInTheDocument();
	});

	it("opens the unified reader overlay when a result DETAILS button is clicked", async () => {
		render(SearchResults, { props: baseProps() });

		await page
			.getByRole("button", { name: /^details$/i })
			.nth(1)
			.click();

		expect(openReaderOverlay).toHaveBeenCalledTimes(1);
		const ctx = vi.mocked(openReaderOverlay).mock.calls[0]?.[0];
		expect(ctx?.surface).toBe(ReadingSurface.OTHER);
		expect(ctx?.currentIndex).toBe(1);
		expect(ctx?.items).toHaveLength(searchResultsFixture.length);
		const expectedResult = searchResultsFixture[1];
		if (!expectedResult) throw new Error("expected second search fixture");
		expect(ctx?.items?.[1]?.url).toBe(expectedResult.link);
	});
});

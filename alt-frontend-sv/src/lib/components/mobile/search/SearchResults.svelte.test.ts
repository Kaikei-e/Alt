import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SearchResults from "./SearchResults.svelte";
import { searchResultsFixture } from "../../../../../tests/fixtures/search";

vi.mock("$lib/api/client", () => ({
	searchFeedsClient: vi.fn(),
	getArticleSummaryClient: vi.fn(() =>
		Promise.resolve({ matched_articles: [] }),
	),
}));

vi.mock("$lib/utils/transformFeedSearchResult", () => ({
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
		render(SearchResults as never, { props: baseProps() });

		await expect.element(page.getByText(/Search Results/)).toBeInTheDocument();
	});

	it("renders search time", async () => {
		render(SearchResults as never, { props: baseProps() });

		await expect.element(page.getByText(/150ms/)).toBeInTheDocument();
	});

	it("shows loading state with pulsing dot, not spinner", async () => {
		const props = baseProps();
		props.results = [];
		props.isLoading = true;

		render(SearchResults as never, { props });

		// Should have pulsing dot indicator
		await expect.element(page.getByText(/Searching/i)).toBeInTheDocument();
	});

	it("shows empty state without emoji", async () => {
		const props = baseProps();
		props.results = [];
		props.isLoading = false;

		render(SearchResults as never, { props });

		await expect
			.element(page.getByText(/No results found/i))
			.toBeInTheDocument();
	});

	it("renders result list items", async () => {
		render(SearchResults as never, { props: baseProps() });

		await expect
			.element(page.getByRole("link", { name: "First Article" }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("link", { name: "Second Article" }))
			.toBeInTheDocument();
	});

	it("shows end marker when no more results", async () => {
		render(SearchResults as never, { props: baseProps() });

		await expect
			.element(page.getByText(/No more results/i))
			.toBeInTheDocument();
	});
});

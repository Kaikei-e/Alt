import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";
import SearchFeedsClient from "./SearchFeedsClient.svelte";

vi.mock("$lib/api/client", () => ({
	searchFeedsClient: vi.fn(() =>
		Promise.resolve({
			results: [],
			error: null,
			next_cursor: null,
			has_more: false,
		}),
	),
	getArticleSummaryClient: vi.fn(() =>
		Promise.resolve({ matched_articles: [] }),
	),
}));

vi.mock("$lib/utils/transformFeedSearchResult", () => ({
	transformFeedSearchResult: vi.fn(() => []),
}));

vi.mock("$app/environment", () => ({
	browser: true,
}));

describe("SearchFeedsClient Alt-Paper compliance", () => {
	it("renders archive desk title", async () => {
		render(SearchFeedsClient as never, { props: {} });

		await expect.element(page.getByText("Search Feeds")).toBeInTheDocument();
	});

	it("renders subtitle", async () => {
		render(SearchFeedsClient as never, { props: {} });

		await expect
			.element(page.getByText(/Search across your RSS feeds/))
			.toBeInTheDocument();
	});

	it("renders search input area", async () => {
		render(SearchFeedsClient as never, { props: {} });

		const input = page.getByRole("textbox", { name: /search query/i });
		await expect.element(input).toBeInTheDocument();
	});

	it("renders tip text without emoji", async () => {
		render(SearchFeedsClient as never, { props: {} });

		await expect
			.element(page.getByText(/Try searching for topics/))
			.toBeInTheDocument();
	});
});

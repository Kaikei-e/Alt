import { page } from "@vitest/browser/context";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";
import SearchFeedsClient from "./SearchFeedsClient.svelte";
import { createMobileSearchSession } from "./search-session";

vi.mock("#lib/api/client/index.js", () => ({
	searchFeedsClient: vi.fn(() =>
		Promise.resolve({
			results: [],
			error: null,
			next_cursor: null,
			has_more: false,
		}),
	),
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

vi.mock("$app/env", () => ({
	browser: true,
}));

/**
 * The query and the rest of the session now belong to the route — they used to
 * be this component's own `$state`, which a rotation destroyed along with the
 * component. Each render here supplies its own so the cases stay independent.
 */
const props = () => ({
	query: "",
	setQuery: () => {},
	session: createMobileSearchSession(),
});

describe("SearchFeedsClient Alt-Paper compliance", () => {
	it("renders archive desk title", async () => {
		render(SearchFeedsClient, { props: props() });

		await expect.element(page.getByText("Search Feeds")).toBeInTheDocument();
	});

	it("renders subtitle", async () => {
		render(SearchFeedsClient, { props: props() });

		await expect
			.element(page.getByText(/Search across your RSS feeds/))
			.toBeInTheDocument();
	});

	it("renders search input area", async () => {
		render(SearchFeedsClient, { props: props() });

		const input = page.getByRole("textbox", { name: /search query/i });
		await expect.element(input).toBeInTheDocument();
	});

	it("renders tip text without emoji", async () => {
		render(SearchFeedsClient, { props: props() });

		await expect
			.element(page.getByText(/Try searching for topics/))
			.toBeInTheDocument();
	});
});

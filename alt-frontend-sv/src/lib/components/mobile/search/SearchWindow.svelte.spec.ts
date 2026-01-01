import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { SearchFeedItem, SearchQuery } from "$lib/schema/search";
import SearchWindow from "./SearchWindow.svelte";

const { mockSearchFeedsClient } = vi.hoisted(() => {
	return { mockSearchFeedsClient: vi.fn() };
});

vi.mock("$lib/api/client", () => ({
	searchFeedsClient: mockSearchFeedsClient,
}));

const baseProps = (): {
	searchQuery: SearchQuery;
	setSearchQuery: (query: SearchQuery) => void;
	setFeedResults: (results: SearchFeedItem[]) => void;
	setCursor: (cursor: string | null) => void;
	setHasMore: (hasMore: boolean) => void;
	isLoading: boolean;
	setIsLoading: (loading: boolean) => void;
} => ({
	searchQuery: { query: "" },
	setSearchQuery: vi.fn(),
	setFeedResults: vi.fn(),
	setCursor: vi.fn(),
	setHasMore: vi.fn(),
	isLoading: false,
	setIsLoading: vi.fn(),
});

describe("SearchWindow", () => {
	beforeEach(() => {
		vi.clearAllMocks();
	});

	it("shows validation error when query is too short", async () => {
		const props = baseProps();
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const { rerender } = render(SearchWindow as any, { props });

		const input = page.getByRole("textbox", { name: /search query/i });
		await input.fill("a");
		await rerender({
			...props,
			searchQuery: { query: "a" },
		});

		const button = page.getByRole("button", { name: /search$/i });
		await button.click();

		await expect
			.element(page.getByText("Search query must be at least 2 characters"))
			.toBeInTheDocument();
		expect(mockSearchFeedsClient).not.toHaveBeenCalled();
	});

	it("calls search API and updates results for valid query", async () => {
		const props = baseProps();
		const resultItem: SearchFeedItem = {
			title: "Svelte 5 release",
			description: "Details about Runes and performance improvements.",
			link: "https://alt.ai/svelte-5",
		};
		mockSearchFeedsClient.mockResolvedValue({
			results: [resultItem],
			error: null,
			next_cursor: 10,
			has_more: true,
		});

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const { rerender } = render(SearchWindow as any, { props });

		const input = page.getByRole("textbox", { name: /search query/i });
		await input.fill("Svelte");
		await rerender({
			...props,
			searchQuery: { query: "Svelte" },
		});

		const button = page.getByRole("button", { name: /search$/i });
		await button.click();

		// Wait for the mock to be called (locators auto-retry)
		await vi.waitFor(() => {
			expect(mockSearchFeedsClient).toHaveBeenCalledWith("Svelte");
		});

		expect(props.setFeedResults).toHaveBeenCalledWith([resultItem]);
		expect(props.setCursor).toHaveBeenCalledWith("10");
		expect(props.setHasMore).toHaveBeenCalledWith(true);
	});
});

import { render, screen, waitFor } from "@testing-library/svelte/svelte5";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type {
	FeedSearchResult,
	SearchFeedItem,
	SearchQuery,
} from "$lib/schema/search";
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
		const { rerender } = render(SearchWindow, { props });

		const input = screen.getByRole("textbox", { name: /search query/i });
		await userEvent.type(input, "a");
		await rerender({
			...props,
			searchQuery: { query: "a" },
		});

		const button = screen.getByRole("button", { name: /search$/i });
		await userEvent.click(button);

		expect(
			screen.getByText("Search query must be at least 2 characters"),
		).toBeInTheDocument();
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

		const { rerender } = render(SearchWindow, { props });

		const input = screen.getByRole("textbox", { name: /search query/i });
		await userEvent.type(input, "Svelte");
		await rerender({
			...props,
			searchQuery: { query: "Svelte" },
		});

		const button = screen.getByRole("button", { name: /search$/i });
		await userEvent.click(button);

		await waitFor(() => {
			expect(mockSearchFeedsClient).toHaveBeenCalledWith("Svelte");
		});

		expect(props.setFeedResults).toHaveBeenCalledWith([resultItem]);
		expect(props.setCursor).toHaveBeenCalledWith("10");
		expect(props.setHasMore).toHaveBeenCalledWith(true);
	});
});

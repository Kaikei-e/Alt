import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { GlobalSearchResult } from "#lib/connect/global_search.js";

const { pageState } = vi.hoisted(() => {
	const value: { url: URL } = {
		url: new URL("http://localhost/search?q=consensus"),
	};
	return {
		pageState: {
			get url() {
				return value.url;
			},
			set url(next: URL) {
				value.url = next;
			},
		},
	};
});

vi.mock("$app/state", () => ({ page: pageState }));

const { gotoMock, searchEverythingMock, createClientTransportMock } =
	vi.hoisted(() => ({
		gotoMock: vi.fn(),
		searchEverythingMock: vi.fn(),
		createClientTransportMock: vi.fn(() => ({})),
	}));

vi.mock("$app/navigation", () => ({
	goto: gotoMock,
}));

vi.mock("#lib/connect/index.js", () => ({
	createClientTransport: createClientTransportMock,
}));

vi.mock("#lib/connect/global_search.js", () => ({
	searchEverything: searchEverythingMock,
}));

import Page from "./+page.svelte";

const SEARCH_RESULT: GlobalSearchResult = {
	query: "consensus",
	searchedAt: "2026-04-07T00:00:00Z",
	articleSection: {
		hits: [
			{
				id: "art-1",
				title: "Raft Consensus Paper",
				snippet: "Understandable distributed consensus",
				link: "https://example.com/raft",
				tags: ["raft"],
				matchedFields: ["title"],
			},
		],
		estimatedTotal: 1,
		hasMore: false,
	},
	recapSection: {
		hits: [],
		estimatedTotal: 0,
		hasMore: false,
	},
	tagSection: {
		hits: [],
		total: 0,
	},
	degradedSections: [],
};

async function settle(ms = 200) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

describe("/search/+page.svelte", () => {
	beforeEach(() => {
		pageState.url = new URL("http://localhost/search?q=consensus");
		gotoMock.mockReset();
		searchEverythingMock.mockReset();
		searchEverythingMock.mockResolvedValue(SEARCH_RESULT);
	});

	it("auto-searches ?q= on mount and clears results when CLEAR is clicked", async () => {
		render(Page);
		await settle(250);

		expect(searchEverythingMock).toHaveBeenCalledWith(
			expect.anything(),
			"consensus",
			undefined,
		);
		await expect
			.element(page.getByText("Raft Consensus Paper"))
			.toBeInTheDocument();

		const clearBtn = page.getByRole("button", { name: "CLEAR" });
		await expect.element(clearBtn).toBeInTheDocument();
		await clearBtn.click();
		await settle(150);

		expect(gotoMock).toHaveBeenCalledWith("/search", {
			replace: true,
			reset: false,
		});
		await expect
			.element(page.getByText("Search your knowledge base"))
			.toBeInTheDocument();
	});

	it("renders degraded sections banner when sections are unavailable", async () => {
		searchEverythingMock.mockResolvedValueOnce({
			...SEARCH_RESULT,
			degradedSections: ["recaps", "tags"],
		});

		render(Page);
		await settle(250);

		const banner = page.getByRole("status");
		await expect.element(banner).toBeInTheDocument();
		await expect
			.element(banner)
			.toHaveTextContent(
				/Some sections are unavailable: recaps, tags\.\s*Results may be incomplete\./,
			);
	});
});

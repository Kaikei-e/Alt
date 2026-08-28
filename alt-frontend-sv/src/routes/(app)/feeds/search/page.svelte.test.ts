/**
 * `/feeds/search` (Archive Desk) across a rotation.
 *
 * Search is the one feed route that is not a `FeedGrid` page: the desk and the
 * phone read a different RPC shape (offset cursors, `SearchFeedItem`) through a
 * different pipeline, so there is no shared grid to hoist. What they *did*
 * share was the bug — the phone's whole session (the query, the results, the
 * offset) lived inside `SearchFeedsClient`, inside the `{#if isDesktop()}`, so
 * turning the phone threw it away and left an empty search box.
 *
 * The fix is the same idea applied to state rather than to markup: the page
 * owns the session, the branch owns only the rendering.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { searchFeedsClient } from "$lib/api/client/feeds";

// Only the leaf module is mocked: `$lib/api/client` re-exports from it, so
// mocking both would give the two import paths two different spies and the
// component under test would call the one the assertions do not watch.
vi.mock("$lib/api/client/feeds", { spy: true });
vi.mock("$app/state", () => ({
	page: { url: new URL("http://localhost/feeds/search") },
}));

import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

const HIT = {
	title: "Svelte 5 runes in anger",
	description: "What changed and what it costs.",
	link: "https://example.com/runes",
	published: "2026-08-01T00:00:00Z",
	article_id: "art-1",
};

async function settle(ms = 250) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

describe("Archive Desk search continuity across a rotation", () => {
	beforeEach(() => {
		vi.mocked(searchFeedsClient).mockReset();
		vi.mocked(searchFeedsClient).mockResolvedValue({
			results: [HIT],
			error: null,
			next_cursor: null,
			has_more: false,
		} as unknown as Awaited<ReturnType<typeof searchFeedsClient>>);
	});

	it("keeps the phone's results and its query across a rotation round trip", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle(300);

		await page.getByTestId("search-input").fill("runes");
		await settle(200);
		await page.getByRole("button", { name: "SEARCH" }).click();
		await settle(500);

		await expect.element(page.getByText(HIT.title)).toBeInTheDocument();
		const searches = vi.mocked(searchFeedsClient).mock.calls.length;

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(400);
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(400);

		// The results survived the trip, and nothing was re-requested to make
		// that true.
		await expect.element(page.getByText(HIT.title)).toBeInTheDocument();
		expect(vi.mocked(searchFeedsClient).mock.calls.length).toBe(searches);
		await expect.element(page.getByTestId("search-input")).toHaveValue("runes");
	});

	it("carries a query typed on the phone into the desk's search box", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle(300);

		await page.getByTestId("search-input").fill("runes");
		await settle(200);

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(400);

		// Two boxes over one query. Rotating mid-search used to hand the reader
		// an empty field and no way to tell what they had been looking for.
		await expect.element(page.getByRole("searchbox")).toHaveValue("runes");
	});
});

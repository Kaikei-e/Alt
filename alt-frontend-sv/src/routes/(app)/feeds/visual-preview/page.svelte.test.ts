/**
 * `/feeds/visual-preview` across a rotation.
 *
 * The page used to hold two `<FeedGrid>`s — one in each arm of an
 * `{#if isDesktop()}` — and `FeedGrid` owns the reader's whole session in
 * local `$state`: the fetched `feeds`, the `removedUrls` set that mark-as-read
 * writes into, and the pagination cursor. Flipping the branch destroyed one
 * grid and built the other from nothing, so a rotation silently re-fetched page
 * one, threw away everything infinite scroll had added, dropped the scroll
 * position along with the DOM, and resurrected every article the reader had
 * marked read.
 *
 * None of that shows up in a snapshot of the markup — both arms render the same
 * *kind* of grid — so these tests follow the live nodes instead: mark the grid
 * element, rotate, and require the mark to still be there. Same technique as
 * `ResponsiveLayout.svelte.test.ts`.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { batchPrefetchImagesClient } from "$lib/api/client/articles";
import {
	getAllFeedsWithCursorClient,
	listSubscriptionsClient,
	updateFeedReadStatusClient,
} from "$lib/api/client/feeds";
import type { RenderFeed } from "$lib/schema/feed";
import { createRenderFeed } from "../../../../../tests/fixtures/feeds";

vi.mock("$lib/api/client/feeds", { spy: true });
vi.mock("$lib/api/client/articles", { spy: true });

// The tiles resolve OG images through a shared resolver that would otherwise
// fire a Connect-RPC at whatever is listening on the dev server.
vi.mock("$lib/utils/ogImageResolver", () => ({
	ogImageResolver: () => ({ resolve: () => Promise.resolve(null) }),
}));

import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

const FEEDS: RenderFeed[] = Array.from({ length: 6 }, (_, i) =>
	createRenderFeed(`feed-${i + 1}`, `https://example.com/feed-${i + 1}`),
);

/** The tile the mark-as-read cases act on. */
const FIRST = FEEDS[0] as RenderFeed;

/** The media-query listener fires off a browser event, so yield a frame. */
async function settle(ms = 250) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * `FeedGrid`'s own root. Identity is the whole point: if the `{#if}` rebuilt
 * the grid, this is a different node and every piece of session state the grid
 * was holding went with the old one.
 */
function gridRoot(): HTMLElement | null {
	return document.querySelector<HTMLElement>(".wire-container");
}

/** One entry per card the grid is currently rendering, at either width. */
function cardLabels(): string[] {
	const grid = gridRoot()?.querySelector('[class*="grid-cols"]');
	if (!grid) return [];
	return Array.from(grid.children).map(
		(child) =>
			child.querySelector("[aria-label]")?.getAttribute("aria-label") ?? "",
	);
}

const openLabel = (feed: RenderFeed) => `Open ${feed.title}`;

function feedFetches(): number {
	return vi.mocked(getAllFeedsWithCursorClient).mock.calls.length;
}

describe("Visual Preview page continuity across a rotation", () => {
	beforeEach(() => {
		vi.mocked(getAllFeedsWithCursorClient).mockReset();
		vi.mocked(getAllFeedsWithCursorClient).mockResolvedValue({
			data: FEEDS,
			next_cursor: undefined,
			has_more: false,
		} as unknown as Awaited<ReturnType<typeof getAllFeedsWithCursorClient>>);

		vi.mocked(listSubscriptionsClient).mockReset();
		vi.mocked(listSubscriptionsClient).mockResolvedValue([]);

		vi.mocked(batchPrefetchImagesClient).mockReset();
		vi.mocked(batchPrefetchImagesClient).mockResolvedValue([]);

		vi.mocked(updateFeedReadStatusClient).mockReset();
		vi.mocked(updateFeedReadStatusClient).mockResolvedValue(
			undefined as unknown as Awaited<
				ReturnType<typeof updateFeedReadStatusClient>
			>,
		);
	});

	it("keeps an article the reader marked read out of the grid after a rotation", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle(400);

		expect(cardLabels()).toHaveLength(FEEDS.length);

		// Open the first tile and mark it read. The grid records the removal in
		// its own `removedUrls`, which is precisely the state a remount loses.
		await page.getByRole("button", { name: openLabel(FIRST) }).click();
		await page.getByRole("button", { name: "Mark as Read" }).click();
		await settle(400);

		expect(cardLabels()).not.toContain(openLabel(FIRST));
		const afterRead = cardLabels().length;

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(400);

		// The loudest symptom of the double grid: an article the reader has
		// already dealt with comes back when they turn the phone.
		expect(cardLabels()).not.toContain(openLabel(FIRST));
		expect(cardLabels()).toHaveLength(afterRead);

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(400);

		expect(cardLabels()).not.toContain(openLabel(FIRST));
		expect(cardLabels()).toHaveLength(afterRead);
	});

	it("keeps the same grid element across a 851 -> 393 -> 851 round trip", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle(400);

		const before = gridRoot();
		expect(before).not.toBeNull();
		(before as HTMLElement).dataset.identityProbe = "landscape";

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(400);
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(400);

		const after = gridRoot();
		expect(after).toBe(before);
		expect(after?.dataset.identityProbe).toBe("landscape");
	});

	it("does not re-fetch the wire, or lose cards, when the phone is rotated", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle(400);

		const fetchesBefore = feedFetches();
		const cardsBefore = cardLabels().length;
		expect(cardsBefore).toBe(FEEDS.length);

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(400);

		expect(cardLabels()).toHaveLength(cardsBefore);
		expect(feedFetches()).toBe(fetchesBefore);

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(400);

		expect(cardLabels()).toHaveLength(cardsBefore);
		expect(feedFetches()).toBe(fetchesBefore);
	});
});

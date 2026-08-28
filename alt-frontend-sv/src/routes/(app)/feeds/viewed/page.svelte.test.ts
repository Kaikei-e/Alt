/**
 * `/feeds/viewed` (The Morgue Desk) across a rotation.
 *
 * The two viewports used to be two entirely separate readers of the same
 * endpoint: a `<FeedGrid>` in the desktop arm and a 356-line `ViewedFeedsClient`
 * in the phone arm, each with its own `feeds` / `cursor` / `hasMore`. Flipping
 * the `{#if}` destroyed one and mounted the other, so a rotation re-fetched the
 * first page from scratch and threw away every page infinite scroll had added.
 *
 * These follow the live nodes rather than the markup: both arms render "a list
 * of filed clippings", so only node identity can tell a preserved list from a
 * rebuilt one.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { getReadFeedsWithCursorClient } from "$lib/api/client/feeds";
import type { RenderFeed } from "$lib/schema/feed";
import { createRenderFeed } from "../../../../../tests/fixtures/feeds";

vi.mock("$lib/api/client/feeds", { spy: true });
vi.mock("$lib/api/client/articles", { spy: true });

import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

const FILED: RenderFeed[] = Array.from({ length: 4 }, (_, i) =>
	createRenderFeed(`filed-${i + 1}`, `https://example.com/filed-${i + 1}`),
);

async function settle(ms = 250) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

function gridRoot(): HTMLElement | null {
	return document.querySelector<HTMLElement>(".wire-container");
}

function cardCount(): number {
	return (
		gridRoot()?.querySelector('[class*="grid-cols"]')?.children.length ?? 0
	);
}

function fetches(): number {
	return vi.mocked(getReadFeedsWithCursorClient).mock.calls.length;
}

describe("Morgue Desk page continuity across a rotation", () => {
	beforeEach(() => {
		vi.mocked(getReadFeedsWithCursorClient).mockReset();
		vi.mocked(getReadFeedsWithCursorClient).mockResolvedValue({
			data: FILED,
			next_cursor: undefined,
			has_more: false,
		} as unknown as Awaited<ReturnType<typeof getReadFeedsWithCursorClient>>);
	});

	it("keeps the same list element across a 851 -> 393 -> 851 round trip", async () => {
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

		expect(gridRoot()).toBe(before);
		expect(gridRoot()?.dataset.identityProbe).toBe("landscape");
	});

	it("does not re-fetch the filings, or lose clippings, when the phone is rotated", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle(400);

		const fetchesBefore = fetches();
		expect(cardCount()).toBe(FILED.length);

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(400);
		expect(cardCount()).toBe(FILED.length);
		expect(fetches()).toBe(fetchesBefore);

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(400);
		expect(cardCount()).toBe(FILED.length);
		expect(fetches()).toBe(fetchesBefore);
	});

	it("keeps the labelled empty region at both widths", async () => {
		vi.mocked(getReadFeedsWithCursorClient).mockResolvedValue({
			data: [],
			next_cursor: undefined,
			has_more: false,
		} as unknown as Awaited<ReturnType<typeof getReadFeedsWithCursorClient>>);

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle(400);

		// The phone list shipped a labelled region with a heading and a sentence
		// saying what will appear here; the desktop grid had a bare line of text.
		// Folding the two lists together must keep the better of the two, not
		// whichever one the desktop happened to have.
		await expect
			.element(page.getByRole("region", { name: /empty morgue state/i }))
			.toBeInTheDocument();

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(400);

		await expect
			.element(page.getByRole("region", { name: /empty morgue state/i }))
			.toBeInTheDocument();
	});
});

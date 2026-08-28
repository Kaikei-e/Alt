import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { removeFavoriteFeedClient } from "$lib/api/client";
import { getFavoriteFeedsWithCursorClient } from "$lib/api/client/feeds";
import type { RenderFeed } from "$lib/schema/feed";

// Spy-mode automocks keep every export bound and let each test override only
// the network calls.
vi.mock("$lib/api/client/feeds", { spy: true });
vi.mock("$lib/api/client", { spy: true });
vi.mock("$lib/api/client/articles", { spy: true });

import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

const clipping: RenderFeed = {
	id: "feed-1",
	title: "A Clipped Dispatch",
	description: "kept for later",
	link: "https://example.com/clipped",
	published: "2026-08-01T00:00:00Z",
	publishedAtFormatted: "Aug 1, 2026",
	mergedTagsLabel: "",
	normalizedUrl: "https://example.com/clipped",
	excerpt: "kept for later",
};

/** The media query listener fires off a browser event, so yield a frame. */
async function settle(ms = 80) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

describe("Clippings File page", () => {
	beforeEach(() => {
		vi.mocked(getFavoriteFeedsWithCursorClient).mockReset();
		vi.mocked(getFavoriteFeedsWithCursorClient).mockResolvedValue({
			data: [clipping],
			next_cursor: undefined,
			has_more: false,
		} as unknown as Awaited<
			ReturnType<typeof getFavoriteFeedsWithCursorClient>
		>);
	});

	it("lists the clippings when the app opens on a phone", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);

		await expect
			.element(page.getByText("A Clipped Dispatch"))
			.toBeInTheDocument();
	});

	it("lists the clippings after a landscape start is rotated upright", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		render(Page);
		await settle();

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();

		// The phone list was fetched in `onMount` behind an `isMobile()` guard,
		// so a session that started in landscape never fetched it — and
		// `mobileIsLoading` starts as `true`, leaving "Retrieving your
		// clippings…" on screen with nothing on its way.
		await expect
			.element(page.getByText("A Clipped Dispatch"))
			.toBeInTheDocument();
	});

	it("keeps the phone list across a round trip instead of dropping back to loading", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await expect
			.element(page.getByText("A Clipped Dispatch"))
			.toBeInTheDocument();

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle();

		// Rotating away and back must not reset the list to its loading state:
		// the fetch is a once-per-side job, not a once-per-rotation one.
		await expect
			.element(page.getByText("A Clipped Dispatch"))
			.toBeInTheDocument();
		await expect
			.element(page.getByText(/Retrieving your clippings/))
			.not.toBeInTheDocument();
	});
});

/**
 * The clippings file across a rotation.
 *
 * The desktop arm of this page rendered a `<FeedGrid>`; the phone arm rendered
 * a second, hand-rolled copy of the same thing directly in the page —
 * `mobileFeeds` / `mobileNextCursor` / `mobileHasMore` plus its own
 * `infiniteScroll`. Flipping the `{#if}` destroyed whichever one was live, so a
 * rotation re-fetched page one and dropped every page the reader had scrolled
 * to, and a clipping they had just removed came back.
 *
 * Node identity is the only way to tell those apart: both arms render "a list
 * of clippings" and the markup looks the same either way.
 */
function gridRoot(): HTMLElement | null {
	return document.querySelector<HTMLElement>(".wire-container");
}

function cardCount(): number {
	return (
		gridRoot()?.querySelector('[class*="grid-cols"]')?.children.length ?? 0
	);
}

const MANY: RenderFeed[] = Array.from({ length: 4 }, (_, i) => ({
	...clipping,
	id: `clip-${i + 1}`,
	title: `Clipping ${i + 1}`,
	link: `https://example.com/clip-${i + 1}`,
	normalizedUrl: `https://example.com/clip-${i + 1}`,
}));

describe("Clippings File continuity across a rotation", () => {
	beforeEach(() => {
		vi.mocked(getFavoriteFeedsWithCursorClient).mockReset();
		vi.mocked(getFavoriteFeedsWithCursorClient).mockResolvedValue({
			data: MANY,
			next_cursor: undefined,
			has_more: false,
		} as unknown as Awaited<
			ReturnType<typeof getFavoriteFeedsWithCursorClient>
		>);
		vi.mocked(removeFavoriteFeedClient).mockReset();
		vi.mocked(removeFavoriteFeedClient).mockResolvedValue(
			undefined as unknown as Awaited<
				ReturnType<typeof removeFavoriteFeedClient>
			>,
		);
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

	it("does not re-fetch the clippings, or lose any, when the phone is rotated", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle(400);

		const fetchesBefore = vi.mocked(getFavoriteFeedsWithCursorClient).mock.calls
			.length;
		expect(cardCount()).toBe(MANY.length);

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(400);
		expect(cardCount()).toBe(MANY.length);
		expect(vi.mocked(getFavoriteFeedsWithCursorClient).mock.calls.length).toBe(
			fetchesBefore,
		);

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(400);
		expect(cardCount()).toBe(MANY.length);
		expect(vi.mocked(getFavoriteFeedsWithCursorClient).mock.calls.length).toBe(
			fetchesBefore,
		);
	});

	it("keeps a removed clipping out of the list after a rotation", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle(400);
		expect(cardCount()).toBe(MANY.length);

		await page
			.getByRole("button", { name: "Remove from clippings" })
			.first()
			.click();
		await settle(400);
		expect(cardCount()).toBe(MANY.length - 1);

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(400);
		expect(cardCount()).toBe(MANY.length - 1);

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(400);
		expect(cardCount()).toBe(MANY.length - 1);
	});
});

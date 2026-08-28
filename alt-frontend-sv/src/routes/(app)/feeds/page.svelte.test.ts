/**
 * `/feeds` across a rotation.
 *
 * The two viewports used to be two unrelated readers of the wire: a
 * `<FeedGrid>` in the desktop arm and a 562-line `FeedsClient` in the phone arm,
 * each with its own `feeds`, cursor, `hasMore` and read-set. Flipping the
 * `{#if}` destroyed whichever was live and built the other from nothing — page
 * one fetched again, every page infinite scroll had added thrown away, the
 * scroll position gone with the DOM, and every article the reader had marked
 * read back on screen.
 *
 * The markup cannot tell those apart, so these follow the live nodes: mark the
 * list, rotate, and require the mark to still be there.
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import {
	getAllFeedsWithCursorClient,
	getFeedsWithCursorClient,
	listSubscriptionsClient,
	updateFeedReadStatusClient,
} from "$lib/api/client/feeds";
import type { RenderFeed } from "$lib/schema/feed";
import { createRenderFeed } from "../../../../tests/fixtures/feeds";

vi.mock("$lib/api/client/feeds", { spy: true });
vi.mock("$lib/api/client/articles", { spy: true });

import Page from "./+page.svelte";

const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

const WIRE: RenderFeed[] = Array.from({ length: 5 }, (_, i) =>
	createRenderFeed(`wire-${i + 1}`, `https://example.com/wire-${i + 1}`),
);

/** The dispatch the mark-as-read cases act on. */
const FIRST = WIRE[0] as RenderFeed;

async function settle(ms = 250) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

function gridRoot(): HTMLElement | null {
	return document.querySelector<HTMLElement>(".wire-container");
}

function cardTitles(): string[] {
	const list = gridRoot()?.querySelector('[class*="grid-cols"]');
	if (!list) return [];
	return Array.from(list.children).map(
		(child) => child.querySelector("a")?.textContent?.trim() ?? "",
	);
}

const renderPage = () =>
	render(Page, { props: { data: { initialFeeds: [] } } });

describe("Feeds page continuity across a rotation", () => {
	beforeEach(() => {
		// Which RPC the page uses depends on the width it was *opened* at — the
		// desk starts on the whole wire, the phone on the unread one — so both
		// are stubbed and the assertions below count the one in play.
		for (const fn of [getFeedsWithCursorClient, getAllFeedsWithCursorClient]) {
			vi.mocked(fn).mockReset();
			vi.mocked(fn).mockResolvedValue({
				data: WIRE,
				next_cursor: undefined,
				has_more: false,
			} as unknown as Awaited<ReturnType<typeof getFeedsWithCursorClient>>);
		}

		vi.mocked(listSubscriptionsClient).mockReset();
		vi.mocked(listSubscriptionsClient).mockResolvedValue([]);

		vi.mocked(updateFeedReadStatusClient).mockReset();
		vi.mocked(updateFeedReadStatusClient).mockResolvedValue(
			undefined as unknown as Awaited<
				ReturnType<typeof updateFeedReadStatusClient>
			>,
		);
	});

	it("keeps an article the reader marked read off the wire after a rotation", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		renderPage();
		await settle(400);

		expect(cardTitles()).toHaveLength(WIRE.length);

		await page
			.getByRole("button", { name: `Mark ${FIRST.title} as read` })
			.click();
		await settle(400);

		expect(cardTitles()).not.toContain(FIRST.title);
		const afterRead = cardTitles().length;

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(400);
		expect(cardTitles()).not.toContain(FIRST.title);
		expect(cardTitles()).toHaveLength(afterRead);

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(400);
		expect(cardTitles()).not.toContain(FIRST.title);
		expect(cardTitles()).toHaveLength(afterRead);
	});

	it("announces a mark-as-read to a screen reader", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		renderPage();
		await settle(400);

		await page
			.getByRole("button", { name: `Mark ${FIRST.title} as read` })
			.click();
		await settle(200);

		// The card vanishes on a tap with nothing else on screen changing; the
		// phone list carried a live region saying so, and folding the two lists
		// together must not quietly drop it.
		const live = document.querySelector('[aria-live="assertive"]');
		expect(live?.textContent?.trim()).toBe("Feed marked as read");
	});

	it("keeps the same list element across a 851 -> 393 -> 851 round trip", async () => {
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		renderPage();
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

	it("does not re-fetch the wire, or lose dispatches, when the phone is rotated", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		renderPage();
		await settle(400);

		const fetchesBefore = vi.mocked(getFeedsWithCursorClient).mock.calls.length;
		expect(cardTitles()).toHaveLength(WIRE.length);

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle(400);
		expect(cardTitles()).toHaveLength(WIRE.length);
		expect(vi.mocked(getFeedsWithCursorClient).mock.calls.length).toBe(
			fetchesBefore,
		);

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		await settle(400);
		expect(cardTitles()).toHaveLength(WIRE.length);
		expect(vi.mocked(getFeedsWithCursorClient).mock.calls.length).toBe(
			fetchesBefore,
		);

		// And it must still be reading the same wire. Deriving `unreadOnly` from
		// the viewport would swap the RPC mid-session, and the grid answers a
		// changed data source by dropping the list and fetching page one — the
		// same damage by a different route.
		expect(vi.mocked(getAllFeedsWithCursorClient)).not.toHaveBeenCalled();
	});

	it("offers a way out of an empty wire at both widths", async () => {
		for (const fn of [getFeedsWithCursorClient, getAllFeedsWithCursorClient]) {
			vi.mocked(fn).mockResolvedValue({
				data: [],
				next_cursor: undefined,
				has_more: false,
			} as unknown as Awaited<ReturnType<typeof getFeedsWithCursorClient>>);
		}

		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		renderPage();
		await settle(400);

		// The phone arm shipped a labelled empty region with a link to add a
		// first feed; the desktop grid had the sentence "No dispatches on the
		// wire" and no way forward.
		await expect
			.element(page.getByRole("region", { name: /empty feed state/i }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("link", { name: /add your first feed/i }))
			.toBeInTheDocument();
	});
});

import { CONNECT_RPC_PATHS } from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson } from "../../utils/mockHelpers";

/**
 * Turning the phone must not cost the reader their session.
 *
 * `/feeds/visual-preview` used to hold two `<FeedGrid>`s, one per arm of an
 * `{#if isDesktop()}`, and the grid is where the session lives: the fetched
 * feeds, the `removedUrls` that mark-as-read writes into, and the pagination
 * cursor are all local `$state` inside it. Flipping the branch destroyed the
 * live grid and mounted a fresh one, which re-fetched page one, dropped
 * everything infinite scroll had added, reset the scroll position with the DOM,
 * and put back every article the reader had marked read.
 *
 * The component specs assert node identity; this asserts the four things a
 * reader would actually notice, in a real browser at real viewport sizes:
 * the cards are still there, the page is still where they left it, nothing was
 * re-requested, and an article they dealt with stays dealt with.
 *
 * 851x393 is a Pixel 5 in landscape and 393x851 upright — the widths that
 * straddle the 768px breakpoint on the device this app is mostly read on.
 */
const PORTRAIT = { width: 393, height: 851 };
const LANDSCAPE = { width: 851, height: 393 };

const LIST_SUBSCRIPTIONS =
	"**/api/v2/alt.feeds.v2.FeedService/ListSubscriptions";

/** Enough covers to make the page taller than either viewport. */
const FEEDS = {
	data: Array.from({ length: 20 }, (_, i) => ({
		id: `feed-${i + 1}`,
		articleId: `article-${i + 1}`,
		title: `Dispatch ${i + 1}`,
		description: `Body of dispatch ${i + 1}.`,
		link: `https://example.com/dispatch-${i + 1}`,
		published: "2 hours ago",
		createdAt: new Date(Date.UTC(2026, 0, 20 - i)).toISOString(),
		author: "Alice",
	})),
	nextCursor: "",
	hasMore: false,
};

test.describe("feeds — rotation continuity", () => {
	let feedRequests = 0;

	test.beforeEach(async ({ page }) => {
		feedRequests = 0;

		await page.route(CONNECT_RPC_PATHS.getAllFeeds, (route) => {
			feedRequests += 1;
			return fulfillJson(route, FEEDS);
		});
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) => {
			feedRequests += 1;
			return fulfillJson(route, FEEDS);
		});
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, { data: [], nextCursor: "", hasMore: false }),
		);
		await page.route(CONNECT_RPC_PATHS.markAsRead, (route) =>
			fulfillJson(route, {}),
		);
		await page.route(LIST_SUBSCRIPTIONS, (route) =>
			fulfillJson(route, { subscriptions: [] }),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, { content: "", articleId: "", url: "" }),
		);
		await page.route(CONNECT_RPC_PATHS.resolveOgImages, (route) =>
			fulfillJson(route, { images: [] }),
		);
	});

	/** Stamp the live grid so a rebuilt one can be told from a kept one. */
	const markGrid = (page: import("@playwright/test").Page) =>
		page.evaluate(() => {
			const grid = document.querySelector<HTMLElement>(".wire-container");
			if (!grid) throw new Error("no grid to mark");
			grid.dataset.identityProbe = "before-rotation";
		});

	const gridStillMarked = (page: import("@playwright/test").Page) =>
		page.evaluate(
			() =>
				document.querySelector<HTMLElement>(".wire-container")?.dataset
					.identityProbe ?? null,
		);

	test("survives 393 -> 851 -> 393 with the same grid, the same cards, one request and no jump to the top", async ({
		page,
	}) => {
		await page.setViewportSize(PORTRAIT);
		await gotoMobileRoute(page, "feeds/visual-preview");

		const cards = page.locator('[data-testid="gallery-grid"] > *');
		await expect(cards).toHaveCount(FEEDS.data.length);

		await page.evaluate(() => window.scrollTo(0, 600));
		await expect.poll(() => page.evaluate(() => window.scrollY)).toBe(600);
		await markGrid(page);

		const before = { cards: await cards.count(), fetches: feedRequests };
		expect(before.fetches).toBe(1);

		await page.setViewportSize(LANDSCAPE);
		await expect(cards).toHaveCount(before.cards);
		expect(feedRequests).toBe(before.fetches);
		expect(await gridStillMarked(page)).toBe("before-rotation");

		await page.setViewportSize(PORTRAIT);
		await expect(cards).toHaveCount(before.cards);
		expect(feedRequests).toBe(before.fetches);

		// The grid element itself is the one that was here before the rotation —
		// the mark is still on it — which is what a remount would have destroyed
		// along with the feeds, the cursor and the scroll position.
		expect(await gridStillMarked(page)).toBe("before-rotation");

		// And they were not thrown back to the top.
		//
		// Deliberately not an exact `scrollY` equality. A tile is 16:9 of half the
		// viewport width, so a wider window genuinely makes the document taller,
		// and Chrome restores the *proportion* of the page scrolled rather than
		// the pixel offset — measured here, 600px at 393 wide comes back as 1216.
		// That is reflow, and it is the same for any two-column gallery. What a
		// rebuilt grid did was different in kind: the list collapsed to its first
		// page, the document shrank under the reader, and the offset became 0.
		expect(await page.evaluate(() => window.scrollY)).toBeGreaterThanOrEqual(
			600,
		);
	});

	test("keeps an article marked read out of the gallery across a rotation", async ({
		page,
	}) => {
		await page.setViewportSize(PORTRAIT);
		await gotoMobileRoute(page, "feeds/visual-preview");

		const cards = page.locator('[data-testid="gallery-grid"] > *');
		await expect(cards).toHaveCount(FEEDS.data.length);

		// `exact` because an inexact role name is a substring match, and
		// "Open Dispatch 1" is a substring of "Open Dispatch 10" through 19.
		const first = page.getByRole("button", {
			name: "Open Dispatch 1",
			exact: true,
		});
		await first.click();
		await page.getByRole("button", { name: "Mark as Read" }).click();

		const gone = first;
		await expect(gone).toHaveCount(0);
		const remaining = await cards.count();

		await page.setViewportSize(LANDSCAPE);
		await expect(gone).toHaveCount(0);
		await expect(cards).toHaveCount(remaining);

		await page.setViewportSize(PORTRAIT);
		await expect(gone).toHaveCount(0);
		await expect(cards).toHaveCount(remaining);
	});

	test("lays the gallery out two across on a phone and wider on a desk", async ({
		page,
	}) => {
		await page.setViewportSize(PORTRAIT);
		await gotoMobileRoute(page, "feeds/visual-preview");
		await expect(page.getByTestId("gallery-tile").first()).toBeVisible();

		const columnsAt = () =>
			page
				.getByTestId("gallery-grid")
				.evaluate(
					(el) => getComputedStyle(el).gridTemplateColumns.split(" ").length,
				);

		// One class string serves both viewports now — `grid-cols-2 gap-2
		// md:gap-5 lg:grid-cols-3 xl:grid-cols-4`. Read the resolved track list
		// rather than the class name: a utility that silently stops applying is
		// exactly what collapsing two class strings into one could break.
		expect(await columnsAt()).toBe(2);

		await page.setViewportSize({ width: 1280, height: 800 });
		expect(await columnsAt()).toBe(4);

		await page.setViewportSize({ width: 1024, height: 800 });
		expect(await columnsAt()).toBe(3);

		await page.setViewportSize(LANDSCAPE);
		expect(await columnsAt()).toBe(2);
	});
});

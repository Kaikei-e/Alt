import {
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson } from "../../utils/mockHelpers";

const SUBSCRIPTIONS_EMPTY = { subscriptions: [] };
const LIST_SUBSCRIPTIONS =
	"**/api/v2/alt.feeds.v2.FeedService/ListSubscriptions";

/**
 * `/feeds/visual-preview` on a phone.
 *
 * The mobile branch of this route used to be a dead end — a paragraph telling
 * the reader to go use the swipe screen instead. It is now a two-column,
 * vertically scrolling gallery of thumbnails, which is the layout NN/g's
 * mobile-list research points to when the image is the thing being scanned
 * (a 2-column grid of images, not a list of tiny thumbnails).
 *
 * Two properties are load-bearing and asserted here rather than left to the
 * component specs, because both only exist once the route, the grid and the
 * real viewport are combined:
 *  - the grid actually resolves to TWO columns at phone width;
 *  - tapping a tile opens the detail modal *in place*, which is what keeps the
 *    reader's scroll position in a list that grows by infinite scroll.
 */
test.describe("mobile feeds — visual preview gallery", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getAllFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(LIST_SUBSCRIPTIONS, (route) =>
			fulfillJson(route, SUBSCRIPTIONS_EMPTY),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, { content: "", articleId: "", url: "" }),
		);
	});

	test("renders the feeds as thumbnail tiles instead of the swipe hand-off", async ({
		page,
	}) => {
		await gotoMobileRoute(page, "feeds/visual-preview");

		const tiles = page.getByTestId("gallery-tile");
		await expect(tiles).toHaveCount(2);
		await expect(tiles.first()).toBeVisible();

		await expect(page.getByText("AI Trends")).toBeVisible();
		await expect(page.getByText("Svelte 5 Tips")).toBeVisible();

		// The old mobile branch was a redirect notice. It must be gone.
		await expect(
			page.getByRole("link", { name: /swipe visual preview/i }),
		).toHaveCount(0);
	});

	test("lays the gallery out in two columns at phone width", async ({
		page,
	}) => {
		await gotoMobileRoute(page, "feeds/visual-preview");
		await expect(page.getByTestId("gallery-tile").first()).toBeVisible();

		// Read the resolved track list rather than trusting the class name: a
		// utility class that silently stops applying is exactly the failure this
		// spec exists to catch.
		const columns = await page
			.getByTestId("gallery-grid")
			.evaluate((el) => getComputedStyle(el).gridTemplateColumns.split(" "));

		expect(columns).toHaveLength(2);
	});

	test("opens the detail modal in place when a tile is tapped", async ({
		page,
	}) => {
		await gotoMobileRoute(page, "feeds/visual-preview");

		await page.getByRole("button", { name: "Open AI Trends" }).click();

		const dialog = page.getByRole("dialog");
		await expect(dialog).toBeVisible();
		await expect(
			dialog.getByRole("heading", { name: "AI Trends" }),
		).toBeVisible();

		// Still on the gallery route — no navigation, so no scroll position lost.
		expect(new URL(page.url()).pathname).toContain("/feeds/visual-preview");
	});
});

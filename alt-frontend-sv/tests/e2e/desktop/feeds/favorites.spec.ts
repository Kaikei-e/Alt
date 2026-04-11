import { test, expect } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";

test.describe("Desktop Clippings File (Favorites)", () => {
	test("renders page title", async ({ page, desktopFavoritesPage }) => {
		await page.route(
			"**/api/v2/alt.feeds.v2.FeedService/GetFavoriteFeeds",
			(route) =>
				fulfillJson(route, { data: [], nextCursor: "", hasMore: false }),
		);

		await desktopFavoritesPage.goto();
		await expect(desktopFavoritesPage.pageTitle).toBeVisible();
	});

	test("shows empty state when no favorites", async ({
		page,
		desktopFavoritesPage,
	}) => {
		await page.route(
			"**/api/v2/alt.feeds.v2.FeedService/GetFavoriteFeeds",
			(route) =>
				fulfillJson(route, { data: [], nextCursor: "", hasMore: false }),
		);

		await desktopFavoritesPage.goto();
		await expect(desktopFavoritesPage.emptyState).toBeVisible();
	});
});

import { test, expect } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_TAG_TRAIL_PATHS,
	CONNECT_TAG_TRAIL_ARTICLES_RESPONSE,
} from "../../fixtures/mockData";

test.describe("Mobile Tag Articles", () => {
	test.use({ viewport: { width: 375, height: 812 } });

	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_TAG_TRAIL_PATHS.fetchArticlesByTag, (route) =>
			fulfillJson(route, CONNECT_TAG_TRAIL_ARTICLES_RESPONSE),
		);
	});

	test("shows tag name in header", async ({ mobileTagArticlesPage }) => {
		await mobileTagArticlesPage.gotoWithTag("AI");
		await mobileTagArticlesPage.waitForArticlesLoaded();
		await expect(mobileTagArticlesPage.pageTitle).toContainText("AI");
	});

	test("renders article list on mobile", async ({ mobileTagArticlesPage }) => {
		await mobileTagArticlesPage.gotoWithTag("AI");
		await mobileTagArticlesPage.waitForArticlesLoaded();
		await expect(mobileTagArticlesPage.articleList).toBeVisible();
	});

	test("shows empty state when no articles", async ({
		page,
		mobileTagArticlesPage,
	}) => {
		await page.route(CONNECT_TAG_TRAIL_PATHS.fetchArticlesByTag, (route) =>
			fulfillJson(route, { articles: [], nextCursor: "", hasMore: false }),
		);
		await mobileTagArticlesPage.gotoWithTag("UnknownTag");
		await mobileTagArticlesPage.waitForArticlesLoaded();
		await expect(mobileTagArticlesPage.emptyState).toBeVisible();
	});
});

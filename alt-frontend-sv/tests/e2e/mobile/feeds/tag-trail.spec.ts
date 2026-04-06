import { test, expect } from "../../fixtures/pomFixtures";
import { fulfillJson, fulfillConnectStream } from "../../utils/mockHelpers";
import {
	CONNECT_TAG_TRAIL_PATHS,
	CONNECT_TAG_TRAIL_FEED_RESPONSE,
	CONNECT_TAG_TRAIL_ARTICLES_RESPONSE,
	CONNECT_RPC_PATHS,
	CONNECT_ARTICLE_CONTENT_RESPONSE,
} from "../../fixtures/mockData";

test.describe("Mobile Tag Trail", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_TAG_TRAIL_PATHS.fetchRandomFeed, (route) =>
			fulfillJson(route, CONNECT_TAG_TRAIL_FEED_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);
		await page.route(CONNECT_TAG_TRAIL_PATHS.fetchArticlesByTag, (route) =>
			fulfillJson(route, CONNECT_TAG_TRAIL_ARTICLES_RESPONSE),
		);
		await page.route(CONNECT_TAG_TRAIL_PATHS.streamArticleTags, (route) =>
			fulfillConnectStream(route, [
				{
					articleId: "article-123",
					tags: [
						{ id: "tag-1", name: "AI", createdAt: "2026-01-01T00:00:00Z" },
						{
							id: "tag-2",
							name: "Machine Learning",
							createdAt: "2026-01-01T00:00:00Z",
						},
					],
					eventType: "EVENT_TYPE_COMPLETED",
				},
			]),
		);
	});

	test("renders feed card", async ({ page, mobileTagTrailPage }) => {
		await mobileTagTrailPage.goto();
		await mobileTagTrailPage.waitForFeedLoaded();
		await expect(page.getByText("Random Trail Feed")).toBeVisible();
	});

	test("tags load after feed", async ({ mobileTagTrailPage }) => {
		await mobileTagTrailPage.goto();
		await mobileTagTrailPage.waitForFeedLoaded();
		await expect(
			mobileTagTrailPage.page.getByRole("button", { name: "AI" }),
		).toBeVisible({ timeout: 10000 });
	});

	test("tag click shows articles", async ({ page, mobileTagTrailPage }) => {
		await mobileTagTrailPage.goto();
		await mobileTagTrailPage.waitForFeedLoaded();
		await expect(page.getByRole("button", { name: "AI" })).toBeVisible({
			timeout: 10000,
		});
		await mobileTagTrailPage.clickTag("AI");
		await expect(page.getByText("AI Trends in 2026")).toBeVisible();
	});

});

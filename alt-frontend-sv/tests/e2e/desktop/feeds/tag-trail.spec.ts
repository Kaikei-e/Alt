import { test, expect } from "../../fixtures/pomFixtures";
import {
	fulfillJson,
	fulfillError,
	fulfillConnectStream,
} from "../../utils/mockHelpers";
import {
	CONNECT_TAG_TRAIL_PATHS,
	CONNECT_TAG_TRAIL_FEED_RESPONSE,
	CONNECT_TAG_TRAIL_ARTICLES_RESPONSE,
	CONNECT_RPC_PATHS,
	CONNECT_ARTICLE_CONTENT_RESPONSE,
} from "../../fixtures/mockData";

test.describe("Desktop Tag Trail", () => {
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
		// Mock streaming tags response
		await page.route(CONNECT_TAG_TRAIL_PATHS.streamArticleTags, (route) =>
			fulfillConnectStream(route, [
				{ kind: "tag", tag: "AI" },
				{ kind: "tag", tag: "Machine Learning" },
				{ kind: "tag", tag: "Technology" },
			]),
		);
	});

	test("renders feed card after loading", async ({
		page,
		desktopTagTrailPage,
	}) => {
		await desktopTagTrailPage.goto();
		await desktopTagTrailPage.waitForFeedLoaded();
		await expect(page.getByText("Random Trail Feed")).toBeVisible();
	});

	test("shows tag buttons after tags stream", async ({
		page,
		desktopTagTrailPage,
	}) => {
		await desktopTagTrailPage.goto();
		await desktopTagTrailPage.waitForFeedLoaded();
		// Wait for tags to appear
		await expect(desktopTagTrailPage.getTagButton("AI")).toBeVisible({
			timeout: 10000,
		});
		await expect(
			desktopTagTrailPage.getTagButton("Machine Learning"),
		).toBeVisible();
		await expect(desktopTagTrailPage.getTagButton("Technology")).toBeVisible();
	});

	test("clicking tag loads articles grid", async ({
		page,
		desktopTagTrailPage,
	}) => {
		await desktopTagTrailPage.goto();
		await desktopTagTrailPage.waitForFeedLoaded();
		await expect(desktopTagTrailPage.getTagButton("AI")).toBeVisible({
			timeout: 10000,
		});

		await desktopTagTrailPage.clickTag("AI");
		await expect(page.getByText("AI Trends in 2026")).toBeVisible();
	});

	test("New Random Feed button reloads", async ({
		page,
		desktopTagTrailPage,
	}) => {
		let requestCount = 0;
		await page.route(CONNECT_TAG_TRAIL_PATHS.fetchRandomFeed, async (route) => {
			requestCount++;
			await fulfillJson(route, CONNECT_TAG_TRAIL_FEED_RESPONSE);
		});

		await desktopTagTrailPage.goto();
		await desktopTagTrailPage.waitForFeedLoaded();
		const initialCount = requestCount;

		await desktopTagTrailPage.refreshButton.click();

		await expect(async () => {
			expect(requestCount).toBeGreaterThan(initialCount);
		}).toPass({ timeout: 5000 });
	});

	test("shows error state on feed fetch failure", async ({
		page,
		desktopTagTrailPage,
	}) => {
		await page.route(CONNECT_TAG_TRAIL_PATHS.fetchRandomFeed, (route) =>
			fulfillError(route, "Server error", 500),
		);

		await desktopTagTrailPage.goto();
		// Should show some error indication
		await expect(
			desktopTagTrailPage.errorMessage
				.or(page.getByText(/error|failed/i))
				.first(),
		).toBeVisible({ timeout: 10000 });
	});
});

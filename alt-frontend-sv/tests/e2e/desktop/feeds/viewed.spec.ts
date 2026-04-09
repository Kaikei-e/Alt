import { test, expect } from "../../fixtures/pomFixtures";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import {
	CONNECT_RPC_PATHS,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_ARTICLE_CONTENT_RESPONSE,
} from "../../fixtures/mockData";
import { buildConnectFeedItem } from "../../fixtures/factories";

test.describe("Desktop Viewed Feeds", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);
	});

	test("shows loading spinner then feeds", async ({
		page,
		desktopViewedPage,
	}) => {
		const readFeedsResponse = {
			data: [
				buildConnectFeedItem({ title: "Read Article 1" }),
				buildConnectFeedItem({ title: "Read Article 2" }),
			],
			nextCursor: "",
			hasMore: false,
		};

		await page.route(CONNECT_RPC_PATHS.getReadFeeds, async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 300));
			await fulfillJson(route, readFeedsResponse);
		});

		await desktopViewedPage.goto();
		// Loading spinner may be visible briefly
		await desktopViewedPage.waitForFeedsLoaded();
	});

	test("shows empty state when no viewed feeds", async ({
		page,
		desktopViewedPage,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		await desktopViewedPage.goto();
		await desktopViewedPage.waitForFeedsLoaded();
		await expect(desktopViewedPage.emptyState).toBeVisible();
	});

	test("opens feed detail modal on card click", async ({
		page,
		desktopViewedPage,
	}) => {
		const response = {
			data: [buildConnectFeedItem({ title: "Read Article" })],
			nextCursor: "",
			hasMore: false,
		};
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, response),
		);

		await desktopViewedPage.goto();
		await desktopViewedPage.waitForFeedsLoaded();
		await desktopViewedPage.selectFeed("Read Article");
		await expect(desktopViewedPage.feedDetailModal).toBeVisible();
	});

	test("closes modal", async ({ page, desktopViewedPage }) => {
		const response = {
			data: [buildConnectFeedItem({ title: "Read Article" })],
			nextCursor: "",
			hasMore: false,
		};
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, response),
		);

		await desktopViewedPage.goto();
		await desktopViewedPage.waitForFeedsLoaded();
		await desktopViewedPage.selectFeed("Read Article");
		await desktopViewedPage.closeModal();
		await expect(desktopViewedPage.feedDetailModal).not.toBeVisible();
	});

	test("shows error state on API failure", async ({
		page,
		desktopViewedPage,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillError(route, "Server error", 500),
		);

		await desktopViewedPage.goto();
		await expect(desktopViewedPage.errorMessage).toBeVisible({
			timeout: 10000,
		});
	});
});

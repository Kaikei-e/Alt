import { buildConnectFeedItem } from "../../fixtures/factories";
import {
	CONNECT_ARTICLE_CONTENT_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillConnectError, fulfillJson } from "../../utils/mockHelpers";

test.describe("Desktop Morgue Desk (Viewed Feeds)", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);
	});

	test("renders populated read feeds when getReadFeeds returns items", async ({
		page,
		desktopViewedPage,
	}) => {
		const readFeedsResponse = {
			data: [
				buildConnectFeedItem({ title: "Populated Read Article 1" }),
				buildConnectFeedItem({ title: "Populated Read Article 2" }),
			],
			nextCursor: "",
			hasMore: false,
		};

		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, readFeedsResponse),
		);

		await desktopViewedPage.goto();
		await desktopViewedPage.waitForFeedsLoaded();

		await expect(desktopViewedPage.getFeedCards()).toHaveCount(2);
		await expect(page.getByText("Populated Read Article 1")).toBeVisible();
		await expect(page.getByText("Populated Read Article 2")).toBeVisible();
	});

	test("shows loading indicator then feeds", async ({
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

		await expect(desktopViewedPage.loadingIndicator).toBeVisible();

		await desktopViewedPage.waitForFeedsLoaded();

		await expect(page.getByText("Read Article 1")).toBeVisible();
		await expect(page.getByText("Read Article 2")).toBeVisible();
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
			fulfillConnectError(route, "Server error"),
		);

		await desktopViewedPage.goto();
		await expect(desktopViewedPage.errorMessage).toBeVisible({
			timeout: 10000,
		});
	});
});

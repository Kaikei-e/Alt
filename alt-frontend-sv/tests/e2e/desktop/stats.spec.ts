import {
	CONNECT_RPC_PATHS,
	CONNECT_TREND_STATS_PATH,
	CONNECT_TREND_STATS_RESPONSE,
} from "../fixtures/mockData";
import { expect, test } from "../fixtures/pomFixtures";
import {
	fulfillConnectStream,
	fulfillConnectStreamError,
	fulfillJson,
} from "../utils/mockHelpers";

const MOCK_STATS = {
	feedAmount: 25,
	unsummarizedFeedAmount: 10,
	articleAmount: 500,
};

const MOCK_STREAM_STATS_MESSAGES = [
	{
		feedAmount: "25",
		unsummarizedFeedAmount: "10",
		totalArticles: "500",
		metadata: {
			timestamp: "1700000000",
			isHeartbeat: false,
		},
	},
];

test.describe("Desktop Statistics", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.streamFeedStats, (route) =>
			fulfillConnectStream(route, MOCK_STREAM_STATS_MESSAGES),
		);
		await page.route(CONNECT_RPC_PATHS.getDetailedFeedStats, (route) =>
			fulfillJson(route, MOCK_STATS),
		);
		await page.route(CONNECT_TREND_STATS_PATH, (route) =>
			fulfillJson(route, CONNECT_TREND_STATS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getUnreadCount, (route) =>
			fulfillJson(route, { count: 42 }),
		);
	});

	test("renders page title and figures bar", async ({ desktopStatsPage }) => {
		await desktopStatsPage.goto();
		await desktopStatsPage.waitForStatsLoaded();

		await expect(desktopStatsPage.pageTitle).toBeVisible();
		await expect(desktopStatsPage.feedsLabel).toBeVisible();
		await expect(desktopStatsPage.articlesLabel).toBeVisible();
		await expect(desktopStatsPage.unsummarizedLabel).toBeVisible();
	});

	test("shows Activity Log heading", async ({ desktopStatsPage }) => {
		await desktopStatsPage.goto();
		await desktopStatsPage.waitForStatsLoaded();

		await expect(desktopStatsPage.activityLogHeading).toBeVisible();
	});

	test("shows reconnect button when disconnected", async ({
		page,
		desktopStatsPage,
	}) => {
		await page.route(CONNECT_RPC_PATHS.streamFeedStats, (route) =>
			fulfillConnectStreamError(route, "unavailable", "Stream disconnected"),
		);

		await desktopStatsPage.goto();
		await expect(desktopStatsPage.pageTitle).toBeVisible();
		await expect(desktopStatsPage.reconnectButton).toBeVisible();
	});
});

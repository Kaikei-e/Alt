import { CONNECT_RPC_PATHS } from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillConnectStream, fulfillJson } from "../../utils/mockHelpers";

// Connect-RPC response format (camelCase)
const CONNECT_STATS_RESPONSE = {
	feedAmount: 12,
	articleAmount: 345,
	unsummarizedFeedAmount: 7,
};

const CONNECT_UNREAD_RESPONSE = {
	count: 42,
};

const MOCK_STREAM_STATS_MESSAGES = [
	{
		feedAmount: "12",
		unsummarizedFeedAmount: "7",
		totalArticles: "345",
		metadata: {
			timestamp: "1700000000",
			isHeartbeat: false,
		},
	},
];

test.describe("mobile feeds routes - stats", () => {
	test("stats page renders counters", async ({ page, mobileStatsPage }) => {
		await page.route(CONNECT_RPC_PATHS.streamFeedStats, (route) =>
			fulfillConnectStream(route, MOCK_STREAM_STATS_MESSAGES),
		);
		await page.route(CONNECT_RPC_PATHS.getDetailedFeedStats, (route) =>
			fulfillJson(route, CONNECT_STATS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getUnreadCount, (route) =>
			fulfillJson(route, CONNECT_UNREAD_RESPONSE),
		);

		await mobileStatsPage.goto();
		await expect(mobileStatsPage.pageTitle).toBeVisible();
		await mobileStatsPage.waitForStatsLoaded();

		await expect(page.getByText("Total Feeds")).toBeVisible();
		await expect(page.getByText("Total Articles")).toBeVisible();
		await expect(page.getByText("Unsummarized")).toBeVisible();
		await expect(page.getByText("Today's Unread")).toBeVisible();
	});
});

import { buildConnectFeedItem } from "../../fixtures/factories";
import {
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";

test.describe("mobile feeds routes - viewed (Morgue Desk)", () => {
	test("viewed page shows empty morgue state", async ({
		page,
		mobileViewedPage,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		await mobileViewedPage.goto();

		await expect(mobileViewedPage.emptyState).toBeVisible();
		await expect(mobileViewedPage.emptyRegion).toBeVisible();
	});

	test("viewed page renders populated read feeds when getReadFeeds returns items", async ({
		page,
		mobileViewedPage,
	}) => {
		const readFeedsResponse = {
			data: [
				buildConnectFeedItem({ title: "Mobile Read Article 1" }),
				buildConnectFeedItem({ title: "Mobile Read Article 2" }),
			],
			nextCursor: "",
			hasMore: false,
		};

		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, readFeedsResponse),
		);

		await mobileViewedPage.goto();

		await expect(mobileViewedPage.feedList).toBeVisible();
		await expect(page.getByText("Mobile Read Article 1")).toBeVisible();
		await expect(page.getByText("Mobile Read Article 2")).toBeVisible();
	});
});

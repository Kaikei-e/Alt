import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";

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
});

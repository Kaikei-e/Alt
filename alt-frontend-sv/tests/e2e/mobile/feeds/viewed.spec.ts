import { expect, test } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson } from "../../utils/mockHelpers";
import { CONNECT_READ_FEEDS_EMPTY_RESPONSE, CONNECT_RPC_PATHS } from "../../fixtures/mockData";

test.describe("mobile feeds routes - viewed", () => {
	test("viewed page shows empty history state", async ({ page }) => {
		// Mock Connect-RPC endpoint
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		await gotoMobileRoute(page, "feeds/viewed");

		await expect(page.getByText("No History Yet")).toBeVisible();
		await expect(page.getByTestId("empty-viewed-feeds-icon")).toBeVisible();
	});
});

import { test, expect } from "../../fixtures/pomFixtures";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import {
	CONNECT_RECAP_RESPONSE,
	CONNECT_RECAP_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";

test.describe("Mobile 3-Day Recap", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecap, (route) =>
			fulfillJson(route, CONNECT_RECAP_RESPONSE),
		);
	});

	test("shows loading skeleton", async ({ page, mobile3DayRecapPage }) => {
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecap, async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 500));
			await fulfillJson(route, CONNECT_RECAP_RESPONSE);
		});

		await mobile3DayRecapPage.goto();
		await expect(mobile3DayRecapPage.skeletonContainer).toBeVisible();
	});

	test("displays recap content after loading", async ({
		page,
		mobile3DayRecapPage,
	}) => {
		await mobile3DayRecapPage.goto();
		await mobile3DayRecapPage.waitForRecapLoaded();

		// Content should be visible (no error)
		await expect(page.locator("body")).not.toContainText("Error loading recap");
	});

	test("shows empty state when no data", async ({
		page,
		mobile3DayRecapPage,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecap, (route) =>
			fulfillJson(route, CONNECT_RECAP_EMPTY_RESPONSE),
		);

		await mobile3DayRecapPage.goto();
		await mobile3DayRecapPage.waitForRecapLoaded();
		await expect(mobile3DayRecapPage.emptyState).toBeVisible({ timeout: 5000 });
	});

	test("shows error with retry button", async ({
		page,
		mobile3DayRecapPage,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecap, (route) =>
			fulfillError(route, "Server error", 500),
		);

		await mobile3DayRecapPage.goto();
		await mobile3DayRecapPage.waitForRecapLoaded();
		await expect(mobile3DayRecapPage.errorMessage).toBeVisible();
		await expect(mobile3DayRecapPage.retryButton).toBeVisible();
	});

	test("retry re-fetches data", async ({ page, mobile3DayRecapPage }) => {
		let requestCount = 0;
		await page.route(CONNECT_RPC_PATHS.getThreeDayRecap, async (route) => {
			requestCount++;
			if (requestCount === 1) {
				await fulfillError(route, "Server error", 500);
			} else {
				await fulfillJson(route, CONNECT_RECAP_RESPONSE);
			}
		});

		await mobile3DayRecapPage.goto();
		await expect(mobile3DayRecapPage.errorMessage).toBeVisible({
			timeout: 15000,
		});

		await mobile3DayRecapPage.retryButton.click();
		await expect(mobile3DayRecapPage.errorMessage).not.toBeVisible({
			timeout: 15000,
		});
	});
});

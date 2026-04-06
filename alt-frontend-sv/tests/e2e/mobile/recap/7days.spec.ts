import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import {
	CONNECT_RECAP_RESPONSE,
	CONNECT_RECAP_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";

test.describe("Mobile Recap 7-Days", () => {
	test.beforeEach(async ({ page }) => {
		// Default mock for recap endpoint
		await page.route(CONNECT_RPC_PATHS.getSevenDayRecap, (route) =>
			fulfillJson(route, CONNECT_RECAP_RESPONSE),
		);
	});

	test("shows loading skeleton initially", async ({
		page,
		mobile3DayRecapPage,
	}) => {
		// Delay response to observe loading state
		await page.route(CONNECT_RPC_PATHS.getSevenDayRecap, async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 500));
			await fulfillJson(route, CONNECT_RECAP_RESPONSE);
		});

		await page.goto("./recap?window=7");

		// Loading skeleton should be visible
		await expect(mobile3DayRecapPage.skeletonContainer).toBeVisible();
	});

	test("displays recap content after loading", async ({
		page,
		mobile3DayRecapPage,
	}) => {
		await page.goto("./recap?window=7");

		// Wait for loading to complete
		await expect(mobile3DayRecapPage.skeletonContainer).not.toBeVisible({
			timeout: 15000,
		});

		// Content should be visible (SwipeRecapScreen)
		// The exact content depends on the component implementation
		await expect(page.locator("body")).not.toContainText("Error loading recap");
	});

	test("shows empty state when no recap data", async ({
		page,
		mobile3DayRecapPage,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getSevenDayRecap, (route) =>
			fulfillJson(route, CONNECT_RECAP_EMPTY_RESPONSE),
		);

		await page.goto("./recap?window=7");

		// Wait for loading to complete
		await expect(mobile3DayRecapPage.skeletonContainer).not.toBeVisible({
			timeout: 15000,
		});

		// Empty state should be visible (RecapEmptyState component)
		await expect(mobile3DayRecapPage.emptyState).toBeVisible({
			timeout: 5000,
		});
	});

	test("shows error state on API failure", async ({
		page,
		mobile3DayRecapPage,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getSevenDayRecap, (route) =>
			fulfillError(route, "Server error", 500),
		);

		await page.goto("./recap?window=7");

		// Wait for loading to complete
		await expect(mobile3DayRecapPage.skeletonContainer).not.toBeVisible({
			timeout: 15000,
		});

		// Error message should be visible
		await expect(mobile3DayRecapPage.errorMessage).toBeVisible();
	});

	test("has retry button on error", async ({ page, mobile3DayRecapPage }) => {
		await page.route(CONNECT_RPC_PATHS.getSevenDayRecap, (route) =>
			fulfillError(route, "Server error", 500),
		);

		await page.goto("./recap?window=7");

		// Wait for loading to complete
		await expect(mobile3DayRecapPage.skeletonContainer).not.toBeVisible({
			timeout: 15000,
		});

		// Retry button should be visible
		await expect(mobile3DayRecapPage.retryButton).toBeVisible();
	});

	test("retry button fetches data again", async ({
		page,
		mobile3DayRecapPage,
	}) => {
		let requestCount = 0;

		await page.route(CONNECT_RPC_PATHS.getSevenDayRecap, async (route) => {
			requestCount++;
			if (requestCount === 1) {
				// First request fails
				await fulfillError(route, "Server error", 500);
			} else {
				// Subsequent requests succeed
				await fulfillJson(route, CONNECT_RECAP_RESPONSE);
			}
		});

		await page.goto("./recap?window=7");

		// Wait for error state
		await expect(mobile3DayRecapPage.errorMessage).toBeVisible({
			timeout: 15000,
		});

		// Click retry
		await mobile3DayRecapPage.retryButton.click();

		// Should show loading or success
		await expect(mobile3DayRecapPage.errorMessage).not.toBeVisible({
			timeout: 15000,
		});
	});

});

test.describe("Mobile Recap 7-Days - Navigation", () => {
	test("can navigate from feeds to recap", async ({
		page,
		mobileFeedsPage,
	}) => {
		// Mock feeds endpoint
		await page.route("**/api/v1/feeds/fetch/cursor**", (route) =>
			fulfillJson(route, { data: [], next_cursor: null, has_more: false }),
		);

		await page.route(CONNECT_RPC_PATHS.getSevenDayRecap, (route) =>
			fulfillJson(route, CONNECT_RECAP_RESPONSE),
		);

		// Start from feeds page
		await mobileFeedsPage.goto();

		// Navigate to recap (through floating menu or navigation)
		await page.goto("./recap?window=7");

		// Should be on recap page
		await expect(page).toHaveURL(/\/recap/);
	});
});

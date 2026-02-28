import { test, expect, devices } from "@playwright/test";
import {
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../fixtures/mockData";

test.use({ ...devices["Pixel 5"] });

/**
 * Visual regression tests for mobile swipe interface.
 */
test.describe("Mobile Swipe Visual Regression", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getAllFeeds, (route) =>
			route.fulfill({
				status: 200,
				contentType: "application/json",
				body: JSON.stringify(CONNECT_FEEDS_RESPONSE),
			}),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			route.fulfill({
				status: 200,
				contentType: "application/json",
				body: JSON.stringify(CONNECT_READ_FEEDS_EMPTY_RESPONSE),
			}),
		);
	});

	test("swipe card layout", async ({ page }) => {
		await page.goto("feeds/swipe");
		await expect(page.locator(".animate-spin").first()).not.toBeVisible({
			timeout: 15000,
		});

		// Wait for swipe card to be visible
		const swipeCard = page.getByTestId("swipe-card");
		if (await swipeCard.isVisible()) {
			await expect(page).toHaveScreenshot("mobile-swipe-card.png", {
				maxDiffPixelRatio: 0.01,
			});
		}
	});
});

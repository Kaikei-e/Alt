import { test, expect } from "@playwright/test";
import {
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../fixtures/mockData";

/**
 * Visual regression tests for desktop feeds page.
 * Uses Playwright's built-in toHaveScreenshot() for snapshot comparison.
 */
test.describe("Desktop Feeds Visual Regression", () => {
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

	test("feed grid layout", async ({ page }) => {
		await page.goto("./feeds");
		await expect(page.locator(".animate-spin").first()).not.toBeVisible({
			timeout: 15000,
		});
		await expect(page.locator(".grid")).toBeVisible();

		await expect(page).toHaveScreenshot("desktop-feeds-grid.png", {
			maxDiffPixelRatio: 0.01,
		});
	});

	test("empty state", async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getAllFeeds, (route) =>
			route.fulfill({
				status: 200,
				contentType: "application/json",
				body: JSON.stringify({ data: [], nextCursor: "", hasMore: false }),
			}),
		);

		await page.goto("./feeds");
		await expect(page.locator(".animate-spin").first()).not.toBeVisible({
			timeout: 15000,
		});

		await expect(page).toHaveScreenshot("desktop-feeds-empty.png", {
			maxDiffPixelRatio: 0.01,
		});
	});
});

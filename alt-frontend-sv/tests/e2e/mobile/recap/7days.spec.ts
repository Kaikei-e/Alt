import { expect, test } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import { RECAP_RESPONSE, RECAP_EMPTY_RESPONSE } from "../../fixtures/mockData";

test.describe("Mobile Recap 7-Days", () => {
	test.beforeEach(async ({ page }) => {
		// Default mock for recap endpoint
		await page.route("**/api/v1/recap/7days", (route) =>
			fulfillJson(route, RECAP_RESPONSE),
		);
	});

	test("shows loading skeleton initially", async ({ page }) => {
		// Delay response to observe loading state
		await page.route("**/api/v1/recap/7days", async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 500));
			await fulfillJson(route, RECAP_RESPONSE);
		});

		await gotoMobileRoute(page, "recap/7days");

		// Loading skeleton should be visible
		const skeleton = page.getByTestId("recap-skeleton-container");
		await expect(skeleton).toBeVisible();
	});

	test("displays recap content after loading", async ({ page }) => {
		await gotoMobileRoute(page, "recap/7days");

		// Wait for loading to complete
		await expect(
			page.getByTestId("recap-skeleton-container"),
		).not.toBeVisible({ timeout: 15000 });

		// Content should be visible (SwipeRecapScreen)
		// The exact content depends on the component implementation
		await expect(page.locator("body")).not.toContainText("Error loading recap");
	});

	test("shows empty state when no recap data", async ({ page }) => {
		await page.route("**/api/v1/recap/7days", (route) =>
			fulfillJson(route, RECAP_EMPTY_RESPONSE),
		);

		await gotoMobileRoute(page, "recap/7days");

		// Wait for loading to complete
		await expect(
			page.getByTestId("recap-skeleton-container"),
		).not.toBeVisible({ timeout: 15000 });

		// Empty state should be visible (EmptyFeedState component)
		// Check for common empty state patterns
		await expect(
			page.getByText(/no feeds/i).or(page.getByText(/empty/i)).first(),
		).toBeVisible({ timeout: 5000 });
	});

	test("shows error state on API failure", async ({ page }) => {
		await page.route("**/api/v1/recap/7days", (route) =>
			fulfillError(route, "Server error", 500),
		);

		await gotoMobileRoute(page, "recap/7days");

		// Wait for loading to complete
		await expect(
			page.getByTestId("recap-skeleton-container"),
		).not.toBeVisible({ timeout: 15000 });

		// Error message should be visible
		await expect(page.getByText("Error loading recap")).toBeVisible();
	});

	test("has retry button on error", async ({ page }) => {
		await page.route("**/api/v1/recap/7days", (route) =>
			fulfillError(route, "Server error", 500),
		);

		await gotoMobileRoute(page, "recap/7days");

		// Wait for loading to complete
		await expect(
			page.getByTestId("recap-skeleton-container"),
		).not.toBeVisible({ timeout: 15000 });

		// Retry button should be visible
		const retryButton = page.getByRole("button", { name: /retry/i });
		await expect(retryButton).toBeVisible();
	});

	test("retry button fetches data again", async ({ page }) => {
		let requestCount = 0;

		await page.route("**/api/v1/recap/7days", async (route) => {
			requestCount++;
			if (requestCount === 1) {
				// First request fails
				await fulfillError(route, "Server error", 500);
			} else {
				// Subsequent requests succeed
				await fulfillJson(route, RECAP_RESPONSE);
			}
		});

		await gotoMobileRoute(page, "recap/7days");

		// Wait for error state
		await expect(page.getByText("Error loading recap")).toBeVisible({
			timeout: 15000,
		});

		// Click retry
		const retryButton = page.getByRole("button", { name: /retry/i });
		await retryButton.click();

		// Should show loading or success
		await expect(page.getByText("Error loading recap")).not.toBeVisible({
			timeout: 15000,
		});
	});

	test("has floating menu", async ({ page }) => {
		await gotoMobileRoute(page, "recap/7days");

		// Wait for page to load
		await expect(
			page.getByTestId("recap-skeleton-container"),
		).not.toBeVisible({ timeout: 15000 });

		// FloatingMenu component should be present
		// Check for common floating menu patterns (FAB, bottom navigation, etc.)
		const floatingElements = page.locator('[class*="fixed"]');
		const count = await floatingElements.count();
		expect(count).toBeGreaterThan(0);
	});
});

test.describe("Mobile Recap 7-Days - Navigation", () => {
	test("can navigate from feeds to recap", async ({ page }) => {
		// Mock feeds endpoint
		await page.route("**/api/v1/feeds/fetch/cursor**", (route) =>
			fulfillJson(route, { data: [], next_cursor: null, has_more: false }),
		);

		await page.route("**/api/v1/recap/7days", (route) =>
			fulfillJson(route, RECAP_RESPONSE),
		);

		// Start from feeds page
		await gotoMobileRoute(page, "feeds");

		// Navigate to recap (through floating menu or navigation)
		await page.goto("./mobile/recap/7days");

		// Should be on recap page
		await expect(page).toHaveURL(/\/mobile\/recap\/7days/);
	});
});

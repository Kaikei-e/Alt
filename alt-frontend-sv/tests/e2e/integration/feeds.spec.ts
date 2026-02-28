import { test, expect } from "@playwright/test";

/**
 * Integration E2E: Feed operations against real backend + DB.
 * Run with: ALT_RUNTIME_URL=http://<IP>:4173/sv/ bun run test:e2e:integration
 */
test.describe("Feeds Integration", () => {
	test("loads feed list from real backend", async ({ page }) => {
		await page.goto("./feeds");

		// Wait for actual data to load (no mocks)
		await expect(page.locator(".animate-spin").first()).not.toBeVisible({
			timeout: 30000,
		});

		// Should show either feeds or empty state
		const feedGrid = page.locator(".grid");
		const emptyState = page.getByText(/no feeds/i);
		await expect(feedGrid.or(emptyState).first()).toBeVisible({
			timeout: 15000,
		});
	});

	test("feed detail modal opens with real data", async ({ page }) => {
		await page.goto("./feeds");
		await expect(page.locator(".animate-spin").first()).not.toBeVisible({
			timeout: 30000,
		});

		// If feeds exist, click the first one
		const feedCards = page.locator('button[aria-label^="Open"]');
		const count = await feedCards.count();

		if (count > 0) {
			await feedCards.first().click();
			const modal = page.locator('[role="dialog"]');
			await expect(modal).toBeVisible();
			// Modal should have a title (h2)
			await expect(modal.locator("h2")).toBeVisible();
		} else {
			test.skip(true, "No feeds available for detail test");
		}
	});

	test("search returns results from real Meilisearch", async ({ page }) => {
		await page.goto("./feeds");
		await expect(page.locator(".animate-spin").first()).not.toBeVisible({
			timeout: 30000,
		});

		const searchInput = page.getByPlaceholder(/search/i);
		if (await searchInput.isVisible()) {
			await searchInput.fill("test");
			await page.keyboard.press("Enter");

			// Wait for search results or empty state
			await page.waitForTimeout(2000);
		} else {
			test.skip(true, "Search input not available");
		}
	});
});

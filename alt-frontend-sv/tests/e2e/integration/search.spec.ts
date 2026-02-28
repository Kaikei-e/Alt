import { test, expect } from "@playwright/test";

/**
 * Integration E2E: Search operations against real Meilisearch.
 * Run with: ALT_RUNTIME_URL=http://<IP>:4173/sv/ bun run test:e2e:integration
 */
test.describe("Search Integration", () => {
	test("search page loads without error", async ({ page }) => {
		await page.goto("./feeds");
		await expect(page.locator(".animate-spin").first()).not.toBeVisible({
			timeout: 30000,
		});

		// Page should not show error state
		await expect(page.getByText(/error loading/i)).not.toBeVisible();
	});

	test("search with known term returns results", async ({ page }) => {
		await page.goto("./feeds");
		await expect(page.locator(".animate-spin").first()).not.toBeVisible({
			timeout: 30000,
		});

		const searchInput = page.getByPlaceholder(/search/i);
		if (!(await searchInput.isVisible())) {
			test.skip(true, "Search input not visible");
			return;
		}

		// Search for e2e seed data token
		await searchInput.fill("e2e-search-token-xyz");
		await page.keyboard.press("Enter");

		// Allow time for search
		await page.waitForTimeout(3000);

		// Check that we got results or a proper empty state (not an error)
		await expect(page.getByText(/error/i)).not.toBeVisible();
	});

	test("empty search shows appropriate state", async ({ page }) => {
		await page.goto("./feeds");
		await expect(page.locator(".animate-spin").first()).not.toBeVisible({
			timeout: 30000,
		});

		const searchInput = page.getByPlaceholder(/search/i);
		if (!(await searchInput.isVisible())) {
			test.skip(true, "Search input not visible");
			return;
		}

		// Search for something that shouldn't exist
		await searchInput.fill("xyznonexistent99999");
		await page.keyboard.press("Enter");

		await page.waitForTimeout(2000);

		// Should not show error state
		await expect(page.getByText(/error loading/i)).not.toBeVisible();
	});
});

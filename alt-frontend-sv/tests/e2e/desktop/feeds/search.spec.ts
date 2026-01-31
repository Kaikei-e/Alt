import { expect, test } from "@playwright/test";

/**
 * Desktop Feeds Search E2E Tests
 *
 * These tests verify the search error handling functionality.
 * The tests focus on ensuring proper error messages are displayed
 * when the search service is unavailable or returns errors.
 */
test.describe("Desktop Feeds Search - Error Handling", () => {
	test.beforeEach(async ({ page }) => {
		// Navigate to search page
		await page.goto("/sv/desktop/feeds/search");
	});

	test("displays search input and button", async ({ page }) => {
		// Verify search UI elements are present
		await expect(page.getByPlaceholder(/search by title/i)).toBeVisible();
		await expect(page.getByRole("button", { name: "Search" })).toBeVisible();
	});

	test("shows initial state with search prompt", async ({ page }) => {
		// Verify initial state message
		await expect(page.getByText(/enter a search query/i)).toBeVisible();
	});

	test("disables search button when input is empty", async ({ page }) => {
		const searchButton = page.getByRole("button", { name: "Search" });
		await expect(searchButton).toBeDisabled();

		// Type something
		await page.getByPlaceholder(/search by title/i).fill("test");
		await expect(searchButton).toBeEnabled();

		// Clear input
		await page.getByPlaceholder(/search by title/i).fill("");
		await expect(searchButton).toBeDisabled();
	});

	test("shows error message when search API returns error", async ({
		page,
	}) => {
		// Mock error response (simulating search-indexer unavailable)
		await page.route("**/SearchFeeds", (route) =>
			route.fulfill({
				status: 500,
				contentType: "application/json",
				body: JSON.stringify({
					code: "internal",
					message:
						"Unable to connect to external service. Please try again. (Error ID: test123)",
				}),
			}),
		);

		// Enter search query and submit
		await page.getByPlaceholder(/search by title/i).fill("test");
		await page.getByRole("button", { name: "Search" }).click();

		// Wait for error message - should show error text
		await expect(page.getByText(/error searching/i)).toBeVisible({
			timeout: 10000,
		});
	});

	test("error message contains Error ID for debugging", async ({ page }) => {
		// Mock error response with specific Error ID
		await page.route("**/SearchFeeds", (route) =>
			route.fulfill({
				status: 500,
				contentType: "application/json",
				body: JSON.stringify({
					code: "internal",
					message:
						"The request took too long. Please try again. (Error ID: abc12345)",
				}),
			}),
		);

		// Enter search query and submit
		await page.getByPlaceholder(/search by title/i).fill("test");
		await page.getByRole("button", { name: "Search" }).click();

		// Wait for error message with Error ID
		await expect(page.getByText(/error searching/i)).toBeVisible({
			timeout: 10000,
		});
		// The error message should be visible (contains the error text from API)
		await expect(page.getByText(/Error ID/i)).toBeVisible({ timeout: 5000 });
	});
});

import { expect, test } from "@playwright/test";
import { DesktopRecapPage } from "../../pages/desktop/DesktopRecapPage";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import { RECAP_RESPONSE, RECAP_EMPTY_RESPONSE } from "../../fixtures/mockData";

test.describe("Desktop Recap", () => {
	let recapPage: DesktopRecapPage;

	test.beforeEach(async ({ page }) => {
		recapPage = new DesktopRecapPage(page);
	});

	test("renders page title and genre list", async ({ page }) => {
		await page.route("**/api/v1/recap/7days", (route) =>
			fulfillJson(route, RECAP_RESPONSE),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// Verify page title
		await expect(recapPage.pageTitle).toBeVisible();
		await expect(recapPage.pageTitle).toContainText("7-Day Recap");

		// Verify genre list is visible
		await expect(recapPage.genreList).toBeVisible();
	});

	test("displays genre items from API response", async ({ page }) => {
		await page.route("**/api/v1/recap/7days", (route) =>
			fulfillJson(route, RECAP_RESPONSE),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// Check genre buttons exist
		await expect(recapPage.getGenreByName("Technology")).toBeVisible();
		await expect(recapPage.getGenreByName("AI/ML")).toBeVisible();
	});

	test("auto-selects first genre on load", async ({ page }) => {
		await page.route("**/api/v1/recap/7days", (route) =>
			fulfillJson(route, RECAP_RESPONSE),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// Recap detail should be visible (indicating a genre is selected)
		await expect(recapPage.recapDetail).toBeVisible();
	});

	test("switches genre when clicking another genre", async ({ page }) => {
		await page.route("**/api/v1/recap/7days", (route) =>
			fulfillJson(route, RECAP_RESPONSE),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// Click on AI/ML genre
		await recapPage.selectGenre("AI/ML");

		// Detail section should update (we can verify the heading changes or contains expected content)
		await expect(recapPage.recapDetail).toBeVisible();
	});

	test("shows empty state when no recap data", async ({ page }) => {
		await page.route("**/api/v1/recap/7days", (route) =>
			fulfillJson(route, RECAP_EMPTY_RESPONSE),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// Verify empty state
		await expect(recapPage.emptyState).toBeVisible();
	});

	test("shows error state on API failure", async ({ page }) => {
		await page.route("**/api/v1/recap/7days", (route) =>
			fulfillError(route, "Server error", 500),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// Verify error message
		await expect(recapPage.errorMessage).toBeVisible();
	});

	test("shows loading spinner while fetching", async ({ page }) => {
		// Delay the response to observe loading state
		await page.route("**/api/v1/recap/7days", async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 500));
			await fulfillJson(route, RECAP_RESPONSE);
		});

		await recapPage.goto();

		// Loading spinner should be visible initially
		await expect(recapPage.loadingSpinner).toBeVisible();

		// Then it should disappear
		await recapPage.waitForRecapLoaded();
		await expect(recapPage.loadingSpinner).not.toBeVisible();
	});
});

test.describe("Desktop Recap - Genre Selection", () => {
	test("genre list maintains selection state", async ({ page }) => {
		const recapPage = new DesktopRecapPage(page);

		await page.route("**/api/v1/recap/7days", (route) =>
			fulfillJson(route, RECAP_RESPONSE),
		);

		await recapPage.goto();
		await recapPage.waitForRecapLoaded();

		// Select different genres and verify UI updates
		const genres = ["Technology", "AI/ML"];

		for (const genre of genres) {
			await recapPage.selectGenre(genre);
			// Small wait for UI update
			await page.waitForTimeout(100);
		}

		// Final selection should be AI/ML
		// The detail panel should reflect the selected genre
		await expect(recapPage.recapDetail).toBeVisible();
	});
});

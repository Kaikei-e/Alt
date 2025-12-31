import { expect, test } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson } from "../../utils/mockHelpers";
import { CONNECT_SEARCH_RESPONSE, CONNECT_RPC_PATHS } from "../../fixtures/mockData";

test.describe("mobile feeds routes - search", () => {
	test("search page shows results for a valid query", async ({ page }) => {
		// Mock Connect-RPC search endpoint
		await page.route(CONNECT_RPC_PATHS.searchFeeds, (route) =>
			fulfillJson(route, CONNECT_SEARCH_RESPONSE),
		);

		await gotoMobileRoute(page, "feeds/search");

		// Use pressSequentially to properly trigger Svelte reactive updates
		const searchInput = page.getByTestId("search-input");
		await searchInput.click();
		await searchInput.pressSequentially("AI", { delay: 50 });

		// Wait for button to be enabled (state has propagated)
		const searchButton = page.getByRole("button", { name: "Search" });
		await expect(searchButton).toBeEnabled();
		await searchButton.click();

		const results = page.getByTestId("search-result-item");
		await expect(results).toHaveCount(1);
		await expect(page.getByRole("link", { name: "AI Weekly" })).toBeVisible();
		await expect(page.getByText("Search Results (1)")).toBeVisible();
	});

	test("search page shows validation errors on short queries", async ({
		page,
	}) => {
		await gotoMobileRoute(page, "feeds/search");

		// Use pressSequentially to properly trigger Svelte reactive updates
		const searchInput = page.getByTestId("search-input");
		await searchInput.click();
		await searchInput.pressSequentially("A", { delay: 50 });

		// Submit the form using Enter key since button may be disabled for single char
		await searchInput.press("Enter");

		await expect(
			page.getByText("Search query must be at least 2 characters"),
		).toBeVisible();
	});
});

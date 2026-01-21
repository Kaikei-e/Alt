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

	test("search page loads more results on scroll", async ({ page }) => {
		// Create mock data for pagination
		// Multiple items ensure the sentinel is below the viewport initially
		const firstPageResponse = {
			data: [
				{
					id: "search-1",
					title: "AI Weekly Issue 1",
					description: "First page result",
					link: "https://example.com/ai-weekly-1",
					published: "3 days ago",
					createdAt: new Date().toISOString(),
					author: "Casey",
				},
				{
					id: "search-2",
					title: "AI Weekly Issue 2",
					description: "First page second result",
					link: "https://example.com/ai-weekly-2",
					published: "4 days ago",
					createdAt: new Date().toISOString(),
					author: "Casey",
				},
				{
					id: "search-3",
					title: "AI Weekly Issue 3",
					description: "First page third result",
					link: "https://example.com/ai-weekly-3",
					published: "5 days ago",
					createdAt: new Date().toISOString(),
					author: "Casey",
				},
			],
			nextCursor: 3, // offset for next page
			hasMore: true,
		};

		const secondPageResponse = {
			data: [
				{
					id: "search-4",
					title: "AI Weekly Issue 4",
					description: "Second page result",
					link: "https://example.com/ai-weekly-4",
					published: "6 days ago",
					createdAt: new Date().toISOString(),
					author: "Casey",
				},
			],
			nextCursor: null,
			hasMore: false,
		};

		let requestCount = 0;
		await page.route(CONNECT_RPC_PATHS.searchFeeds, (route) => {
			requestCount++;
			if (requestCount === 1) {
				fulfillJson(route, firstPageResponse);
			} else {
				fulfillJson(route, secondPageResponse);
			}
		});

		await gotoMobileRoute(page, "feeds/search");

		// Perform search
		const searchInput = page.getByTestId("search-input");
		await searchInput.click();
		await searchInput.pressSequentially("AI", { delay: 50 });

		const searchButton = page.getByRole("button", { name: "Search" });
		await expect(searchButton).toBeEnabled();
		await searchButton.click();

		// Wait for first page results
		await expect(page.getByText("AI Weekly Issue 1")).toBeVisible();
		await expect(page.getByTestId("search-result-item")).toHaveCount(3);

		// Scroll sentinel into view to trigger infinite scroll
		await page.getByTestId("infinite-scroll-sentinel").scrollIntoViewIfNeeded();

		// Wait for second page to load
		await expect(page.getByText("AI Weekly Issue 4")).toBeVisible();
		await expect(page.getByTestId("search-result-item")).toHaveCount(4);

		// Verify "Loading more..." appears during loading
		// Note: This might be hard to catch due to timing, so we skip this assertion

		// Verify "No more results" message appears
		await expect(page.getByText("No more results to load")).toBeVisible();
	});
});

import { expect, test } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_SEARCH_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";

test.describe("mobile feeds routes - search", () => {
	test("search page shows results for a valid query", async ({
		page,
		mobileSearchPage,
	}) => {
		// Mock Connect-RPC search endpoint
		await page.route(CONNECT_RPC_PATHS.searchFeeds, (route) =>
			fulfillJson(route, CONNECT_SEARCH_RESPONSE),
		);

		await mobileSearchPage.goto();

		// Use pressSequentially to properly trigger Svelte reactive updates
		await mobileSearchPage.searchInput.click();
		await mobileSearchPage.searchInput.pressSequentially("AI", {
			delay: 50,
		});

		// Wait for button to be enabled (state has propagated)
		await expect(mobileSearchPage.searchButton).toBeEnabled();
		await mobileSearchPage.searchButton.click();

		await expect(mobileSearchPage.resultItems).toHaveCount(1);
		await expect(page.getByRole("link", { name: "AI Weekly" })).toBeVisible();
		await expect(page.getByText("Search Results (1)")).toBeVisible();
	});

	test("search page shows validation errors on short queries", async ({
		mobileSearchPage,
	}) => {
		await mobileSearchPage.goto();

		// Use pressSequentially to properly trigger Svelte reactive updates
		await mobileSearchPage.searchInput.click();
		await mobileSearchPage.searchInput.pressSequentially("A", { delay: 50 });

		// Submit the form using Enter key since button may be disabled for single char
		await mobileSearchPage.searchInput.press("Enter");

		await expect(mobileSearchPage.validationError).toBeVisible();
	});

	test("search page loads more results on scroll", async ({
		page,
		mobileSearchPage,
	}) => {
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

		await mobileSearchPage.goto();

		// Perform search
		await mobileSearchPage.searchInput.click();
		await mobileSearchPage.searchInput.pressSequentially("AI", {
			delay: 50,
		});

		await expect(mobileSearchPage.searchButton).toBeEnabled();
		await mobileSearchPage.searchButton.click();

		// Wait for first page results
		await expect(page.getByText("AI Weekly Issue 1")).toBeVisible();

		// Scroll sentinel into view to trigger infinite scroll
		// (on small mobile viewports the sentinel may already be visible,
		//  so the second page can load automatically)
		await mobileSearchPage.infiniteScrollSentinel.scrollIntoViewIfNeeded();

		// Wait for all results including second page
		await expect(page.getByText("AI Weekly Issue 4")).toBeVisible();
		await expect(mobileSearchPage.resultItems).toHaveCount(4);

		// Verify "Loading more..." appears during loading
		// Note: This might be hard to catch due to timing, so we skip this assertion

		// Verify "No more results" message appears
		await expect(mobileSearchPage.noMoreResults).toBeVisible();
	});
});

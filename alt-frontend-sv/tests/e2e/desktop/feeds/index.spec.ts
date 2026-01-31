import { expect, test } from "@playwright/test";
import { DesktopFeedsPage } from "../../pages/desktop/DesktopFeedsPage";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_FEEDS_RESPONSE,
	CONNECT_FEEDS_EMPTY_RESPONSE,
	CONNECT_FEEDS_WITHOUT_ARTICLE_ID,
	CONNECT_FEEDS_NAVIGATION_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_MARK_AS_READ_RESPONSE,
	CONNECT_RPC_PATHS,
	CONNECT_ARTICLE_CONTENT_RESPONSE,
} from "../../fixtures/mockData";

test.describe("Desktop Feeds", () => {
	let feedsPage: DesktopFeedsPage;

	test.beforeEach(async ({ page }) => {
		feedsPage = new DesktopFeedsPage(page);

		// Mock Connect-RPC endpoints
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);

		// Mock read feeds (empty)
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		// Mock article content (auto-fetched on modal open)
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);
	});

	test("renders feed grid with cards", async () => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Verify page title
		await expect(feedsPage.pageTitle).toBeVisible();

		// Verify feeds are displayed
		const feedCount = await feedsPage.getFeedCount();
		expect(feedCount).toBe(2);
	});

	test("displays feed card titles correctly", async () => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Check specific feed titles exist
		await expect(feedsPage.getFeedCardByTitle("AI Trends")).toBeVisible();
		await expect(feedsPage.getFeedCardByTitle("Svelte 5 Tips")).toBeVisible();
	});

	test("opens feed detail modal on card click", async ({ page }) => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Click on a feed card
		await feedsPage.selectFeed("AI Trends");

		// Verify modal is open with correct content
		await expect(feedsPage.feedDetailModal).toBeVisible();
		await feedsPage.expectModalTitle("AI Trends");

		// Verify action buttons are present
		await expect(feedsPage.markAsReadButton).toBeVisible();
		// Article button shows "Full Article" or "Article Loaded" depending on auto-fetch state
		await expect(
			page.getByRole("button", { name: /full article|article loaded/i }),
		).toBeVisible();
	});

	test("closes feed detail modal", async () => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Open and close modal
		await feedsPage.selectFeed("AI Trends");
		await expect(feedsPage.feedDetailModal).toBeVisible();

		await feedsPage.closeModal();
		await expect(feedsPage.feedDetailModal).not.toBeVisible();
	});

	test("marks feed as read and navigates to next feed", async ({ page }) => {
		// Mock mark as read endpoint (Connect-RPC)
		await page.route(CONNECT_RPC_PATHS.markAsRead, (route) =>
			fulfillJson(route, CONNECT_MARK_AS_READ_RESPONSE),
		);

		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Get initial feed count
		const initialCount = await feedsPage.getFeedCount();

		// Open first feed and mark as read
		await feedsPage.selectFeed("AI Trends");
		await feedsPage.expectModalTitle("AI Trends");

		// Mark as read - should navigate to next feed (Svelte 5 Tips)
		await feedsPage.markAsReadButton.click();

		// Modal should still be visible but showing next feed
		await expect(feedsPage.feedDetailModal).toBeVisible();
		await feedsPage.expectModalTitle("Svelte 5 Tips");

		// Feed count should decrease by 1
		const newCount = await feedsPage.getFeedCount();
		expect(newCount).toBe(initialCount - 1);
	});

	test("marks last feed as read and closes modal", async ({ page }) => {
		// Override the feeds mock to prevent infinite scroll from fetching more
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, {
				...CONNECT_FEEDS_RESPONSE,
				hasMore: false, // Prevent infinite scroll from loading more
				nextCursor: "",
			}),
		);

		// Mock mark as read endpoint (Connect-RPC)
		await page.route(CONNECT_RPC_PATHS.markAsRead, (route) =>
			fulfillJson(route, CONNECT_MARK_AS_READ_RESPONSE),
		);

		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Open last feed (Svelte 5 Tips is second and last)
		await feedsPage.selectFeed("Svelte 5 Tips");
		await feedsPage.expectModalTitle("Svelte 5 Tips");

		// Mark as read - should close modal since it's the last feed
		await feedsPage.markAsReadButton.click();

		// Modal should close
		await expect(feedsPage.feedDetailModal).not.toBeVisible();
	});

	test("shows empty state when no feeds", async ({ page }) => {
		// Override with empty response (Connect-RPC)
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_EMPTY_RESPONSE),
		);

		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Verify empty state
		await expect(feedsPage.emptyState).toBeVisible();
	});

	test("shows error state on API failure", async ({ page }) => {
		// Mock error response (Connect-RPC)
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, { code: "internal", message: "Server error" }, 500),
		);

		await feedsPage.goto();

		// Wait for loading to complete
		await expect(feedsPage.loadingSpinner).not.toBeVisible({ timeout: 15000 });

		// Verify error message
		await expect(feedsPage.errorMessage).toBeVisible();
	});

	test("auto-fetches article content when modal opens", async () => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Open modal - article should be auto-fetched (mock is set in beforeEach)
		await feedsPage.selectFeed("AI Trends");

		// Wait for button state to change (showing "Article Loaded") without clicking
		await expect(
			feedsPage.page.getByRole("button", { name: /article loaded/i }),
		).toBeVisible({ timeout: 10000 });
	});

	test("mark as read is always enabled regardless of articleId", async ({
		page,
	}) => {
		// Override feeds with feed that has no articleId (not saved)
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_WITHOUT_ARTICLE_ID),
		);

		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Open modal - Mark as Read should be immediately available
		await feedsPage.selectFeed("AI Trends");
		await expect(
			page.getByRole("button", { name: /mark as read/i }),
		).toBeVisible();
		await expect(
			page.getByRole("button", { name: /mark as read/i }),
		).toBeEnabled();
	});

	test("displays feed grid with 3 columns on large screens", async ({
		page,
	}) => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Ensure viewport is large screen (lg breakpoint: 1024px+)
		await page.setViewportSize({ width: 1280, height: 800 });

		// Get the feed grid element
		const feedGrid = page.locator(".grid").first();

		// Verify grid has lg:grid-cols-3 class (not lg:grid-cols-4)
		await expect(feedGrid).toHaveClass(/lg:grid-cols-3/);
	});
});

test.describe("Desktop Feeds - Accessibility", () => {
	test("feed cards have accessible labels", async ({ page }) => {
		const feedsPage = new DesktopFeedsPage(page);

		// Mock Connect-RPC endpoints
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Verify cards have aria-labels
		const cards = feedsPage.getFeedCards();
		const count = await cards.count();

		for (let i = 0; i < count; i++) {
			const ariaLabel = await cards.nth(i).getAttribute("aria-label");
			expect(ariaLabel).toMatch(/^Open .+$/);
		}
	});
});

test.describe("Desktop Feeds - Modal Navigation", () => {
	let feedsPage: DesktopFeedsPage;

	test.beforeEach(async ({ page }) => {
		feedsPage = new DesktopFeedsPage(page);

		// Mock with 3 feeds for navigation testing
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_NAVIGATION_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		// Mock article content
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);
	});

	test("shows next arrow but not previous arrow on first feed", async () => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Click on first feed
		await feedsPage.selectFeed("First Feed");
		await feedsPage.expectModalTitle("First Feed");

		// Should show next arrow but not previous
		await expect(feedsPage.nextFeedButton).toBeVisible();
		await expect(feedsPage.prevFeedButton).not.toBeVisible();
	});

	test("shows both arrows on middle feed", async () => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Click on second feed
		await feedsPage.selectFeed("Second Feed");
		await feedsPage.expectModalTitle("Second Feed");

		// Should show both arrows
		await expect(feedsPage.prevFeedButton).toBeVisible();
		await expect(feedsPage.nextFeedButton).toBeVisible();
	});

	test("shows previous arrow but not next arrow on last feed", async () => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Click on last feed
		await feedsPage.selectFeed("Third Feed");
		await feedsPage.expectModalTitle("Third Feed");

		// Should show previous arrow but not next
		await expect(feedsPage.prevFeedButton).toBeVisible();
		await expect(feedsPage.nextFeedButton).not.toBeVisible();
	});

	test("navigates to next feed using arrow button", async () => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Open first feed
		await feedsPage.selectFeed("First Feed");
		await feedsPage.expectModalTitle("First Feed");

		// Click next arrow
		await feedsPage.navigateToNextFeed();

		// Should now show second feed
		await feedsPage.expectModalTitle("Second Feed");
	});

	test("navigates to previous feed using arrow button", async () => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Open second feed
		await feedsPage.selectFeed("Second Feed");
		await feedsPage.expectModalTitle("Second Feed");

		// Click previous arrow
		await feedsPage.navigateToPreviousFeed();

		// Should now show first feed
		await feedsPage.expectModalTitle("First Feed");
	});

	test("navigates to next feed using keyboard arrow right", async () => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Open first feed
		await feedsPage.selectFeed("First Feed");
		await feedsPage.expectModalTitle("First Feed");

		// Press right arrow
		await feedsPage.navigateToNextFeedWithKeyboard();

		// Should now show second feed
		await feedsPage.expectModalTitle("Second Feed");
	});

	test("navigates to previous feed using keyboard arrow left", async () => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Open second feed
		await feedsPage.selectFeed("Second Feed");
		await feedsPage.expectModalTitle("Second Feed");

		// Press left arrow
		await feedsPage.navigateToPreviousFeedWithKeyboard();

		// Should now show first feed
		await feedsPage.expectModalTitle("First Feed");
	});

	test("can navigate through all feeds sequentially", async () => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Open first feed
		await feedsPage.selectFeed("First Feed");
		await feedsPage.expectModalTitle("First Feed");

		// Navigate to second
		await feedsPage.navigateToNextFeed();
		await feedsPage.expectModalTitle("Second Feed");

		// Navigate to third
		await feedsPage.navigateToNextFeed();
		await feedsPage.expectModalTitle("Third Feed");

		// Should not have next arrow on last feed
		await expect(feedsPage.nextFeedButton).not.toBeVisible();

		// Navigate back to second
		await feedsPage.navigateToPreviousFeed();
		await feedsPage.expectModalTitle("Second Feed");

		// Navigate back to first
		await feedsPage.navigateToPreviousFeed();
		await feedsPage.expectModalTitle("First Feed");

		// Should not have previous arrow on first feed
		await expect(feedsPage.prevFeedButton).not.toBeVisible();
	});

	test("prefetches next 2 articles when modal opens", async ({ page }) => {
		const fetchRequests: string[] = [];

		// Monitor all requests to the article content endpoint
		page.on("request", (request) => {
			if (request.url().includes("FetchArticleContent")) {
				fetchRequests.push(request.url());
			}
		});

		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Open first feed - should trigger prefetch of next 2 articles
		await feedsPage.selectFeed("First Feed");
		await feedsPage.expectModalTitle("First Feed");

		// Wait for prefetch to complete (500ms delay * 2 + fetch time)
		await page.waitForTimeout(2000);

		// Should have fetched: current (1st) + prefetched (2nd, 3rd)
		expect(fetchRequests.length).toBeGreaterThanOrEqual(3);
	});
});

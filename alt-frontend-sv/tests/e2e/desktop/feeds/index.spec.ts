import { expect, test } from "@playwright/test";
import { DesktopFeedsPage } from "../../pages/desktop/DesktopFeedsPage";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	FEEDS_RESPONSE,
	FEEDS_EMPTY_RESPONSE,
	MARK_AS_READ_RESPONSE,
	ARTICLE_CONTENT_RESPONSE,
} from "../../fixtures/mockData";

test.describe("Desktop Feeds", () => {
	let feedsPage: DesktopFeedsPage;

	test.beforeEach(async ({ page }) => {
		feedsPage = new DesktopFeedsPage(page);

		// Default mock for feeds endpoint
		await page.route("**/api/v1/feeds/fetch/cursor**", (route) =>
			fulfillJson(route, FEEDS_RESPONSE),
		);

		// Mock viewed feeds (empty)
		await page.route("**/api/v1/feeds/fetch/viewed/cursor**", (route) =>
			fulfillJson(route, FEEDS_EMPTY_RESPONSE),
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

	test("opens feed detail modal on card click", async () => {
		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Click on a feed card
		await feedsPage.selectFeed("AI Trends");

		// Verify modal is open with correct content
		await expect(feedsPage.feedDetailModal).toBeVisible();
		await feedsPage.expectModalTitle("AI Trends");

		// Verify action buttons are present
		await expect(feedsPage.markAsReadButton).toBeVisible();
		await expect(feedsPage.fullArticleButton).toBeVisible();
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

	test("marks feed as read and closes modal", async ({ page }) => {
		// Mock mark as read endpoint
		await page.route("**/api/v1/feeds/read", (route) =>
			fulfillJson(route, MARK_AS_READ_RESPONSE),
		);

		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Get initial feed count
		const initialCount = await feedsPage.getFeedCount();

		// Open modal and mark as read
		await feedsPage.selectFeed("AI Trends");
		await feedsPage.markCurrentFeedAsRead();

		// Modal should close
		await expect(feedsPage.feedDetailModal).not.toBeVisible();

		// Feed count should decrease by 1
		const newCount = await feedsPage.getFeedCount();
		expect(newCount).toBe(initialCount - 1);
	});

	test("shows empty state when no feeds", async ({ page }) => {
		// Override with empty response
		await page.route("**/api/v1/feeds/fetch/cursor**", (route) =>
			fulfillJson(route, FEEDS_EMPTY_RESPONSE),
		);

		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Verify empty state
		await expect(feedsPage.emptyState).toBeVisible();
	});

	test("shows error state on API failure", async ({ page }) => {
		// Mock error response
		await page.route("**/api/v1/feeds/fetch/cursor**", (route) =>
			fulfillJson(route, { error: "Server error" }, 500),
		);

		await feedsPage.goto();

		// Wait for loading to complete
		await expect(feedsPage.loadingSpinner).not.toBeVisible({ timeout: 15000 });

		// Verify error message
		await expect(feedsPage.errorMessage).toBeVisible();
	});

	test("loads full article in modal", async ({ page }) => {
		// Mock article content endpoint
		await page.route("**/api/v1/articles/content**", (route) =>
			fulfillJson(route, ARTICLE_CONTENT_RESPONSE),
		);

		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();

		// Open modal
		await feedsPage.selectFeed("AI Trends");

		// Click full article button
		await feedsPage.fullArticleButton.click();

		// Wait for button state to change (showing "Article Loaded")
		await expect(
			page.getByRole("button", { name: /article loaded/i }),
		).toBeVisible({ timeout: 10000 });
	});
});

test.describe("Desktop Feeds - Accessibility", () => {
	test("feed cards have accessible labels", async ({ page }) => {
		const feedsPage = new DesktopFeedsPage(page);

		await page.route("**/api/v1/feeds/fetch/cursor**", (route) =>
			fulfillJson(route, FEEDS_RESPONSE),
		);
		await page.route("**/api/v1/feeds/fetch/viewed/cursor**", (route) =>
			fulfillJson(route, FEEDS_EMPTY_RESPONSE),
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

/**
 * Mobile Swipe Feed E2E Tests (Page Object Model)
 *
 * Tests for the swipe feed interface using Page Object Model pattern.
 * This demonstrates best practices for maintainable E2E tests.
 */
import { expect, test } from "@playwright/test";
import { MobileSwipePage } from "../../pages/mobile/MobileSwipePage";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import {
	CONNECT_RPC_PATHS,
	CONNECT_ARTICLE_CONTENT_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_MARK_AS_READ_RESPONSE,
} from "../../fixtures/mockData";

// Test data
const MOCK_FEEDS = {
	data: [
		{
			id: "feed-1",
			title: "Test Article One",
			description: "Description for first test article.",
			link: "https://example.com/article-1",
			published: "2025-01-15",
			createdAt: new Date().toISOString(),
			author: "Test Author",
		},
		{
			id: "feed-2",
			title: "Test Article Two",
			description: "Description for second test article.",
			link: "https://example.com/article-2",
			published: "2025-01-14",
			createdAt: new Date().toISOString(),
			author: "Another Author",
		},
	],
	nextCursor: "next-cursor",
	hasMore: true,
};

const EMPTY_FEEDS = {
	data: [],
	nextCursor: "",
	hasMore: false,
};

test.describe("Mobile Swipe Feed - Page Object Model Tests", () => {
	let swipePage: MobileSwipePage;

	test.beforeEach(async ({ page }) => {
		swipePage = new MobileSwipePage(page);

		// Setup default mocks
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, MOCK_FEEDS),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);
	});

	test.describe("page rendering", () => {
		test("displays swipe card with article title", async () => {
			await swipePage.goto();
			await swipePage.waitForPageReady();

			const title = await swipePage.getCardTitle();
			expect(title).toContain("Test Article One");
		});

		test("displays action footer with buttons", async () => {
			await swipePage.goto();
			await swipePage.waitForPageReady();

			const isFooterVisible = await swipePage.isFooterVisible();
			expect(isFooterVisible).toBe(true);

			await expect(swipePage.articleButton).toBeVisible();
			await expect(swipePage.summaryButton).toBeVisible();
		});

		test("external link has correct URL", async () => {
			await swipePage.goto();
			await swipePage.waitForPageReady();

			const link = swipePage.getExternalLink();
			await expect(link).toHaveAttribute(
				"href",
				"https://example.com/article-1",
			);
		});
	});

	test.describe("accessibility", () => {
		test("swipe card has correct accessibility attributes", async () => {
			await swipePage.goto();
			await swipePage.waitForPageReady();

			await swipePage.assertAccessibility();
		});

		test("buttons have accessible names", async () => {
			await swipePage.goto();
			await swipePage.waitForPageReady();

			await expect(swipePage.articleButton).toHaveAccessibleName(/article/i);
			await expect(swipePage.summaryButton).toHaveAccessibleName(/summary/i);
		});
	});

	test.describe("swipe interactions", () => {
		test("swipe left marks article as read", async ({ page }) => {
			let markAsReadCalled = false;
			await page.route(CONNECT_RPC_PATHS.markAsRead, (route) => {
				markAsReadCalled = true;
				return fulfillJson(route, CONNECT_MARK_AS_READ_RESPONSE);
			});

			await swipePage.goto();
			await swipePage.waitForPageReady();

			await swipePage.swipeLeft();

			await expect.poll(() => markAsReadCalled, { timeout: 5000 }).toBe(true);
		});

		test("swipe right marks article as read", async ({ page }) => {
			let markAsReadCalled = false;
			await page.route(CONNECT_RPC_PATHS.markAsRead, (route) => {
				markAsReadCalled = true;
				return fulfillJson(route, CONNECT_MARK_AS_READ_RESPONSE);
			});

			await swipePage.goto();
			await swipePage.waitForPageReady();

			await swipePage.swipeRight();

			await expect.poll(() => markAsReadCalled, { timeout: 5000 }).toBe(true);
		});
	});

	test.describe("content expansion", () => {
		test("clicking Article button shows content section", async () => {
			await swipePage.goto();
			await swipePage.waitForPageReady();

			await swipePage.toggleArticleContent();
			await swipePage.waitForContentLoaded();

			await expect(swipePage.contentSection).toBeVisible();
		});

		test("clicking Article button twice hides content section", async () => {
			await swipePage.goto();
			await swipePage.waitForPageReady();

			// Expand
			await swipePage.toggleArticleContent();
			await swipePage.waitForContentLoaded();
			await expect(swipePage.contentSection).toBeVisible();

			// Collapse
			await swipePage.toggleArticleContent();
			await expect(swipePage.contentSection).not.toBeVisible();
		});
	});

	test.describe("empty state", () => {
		test("handles empty feed list gracefully", async ({ page }) => {
			// Override with empty feeds
			await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
				fulfillJson(route, EMPTY_FEEDS),
			);

			await swipePage.goto();

			// Should show some empty state or message
			// The exact behavior depends on implementation
			await page.waitForLoadState("networkidle");
		});
	});

	test.describe("error handling", () => {
		test("handles API error gracefully", async ({ page }) => {
			await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
				fulfillError(route, "Internal Server Error", 500),
			);

			await swipePage.goto();

			// Should handle error state gracefully
			await page.waitForLoadState("networkidle");
		});

		test("handles article content fetch error", async ({ page }) => {
			await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
				fulfillError(route, "Not Found", 404),
			);

			await swipePage.goto();
			await swipePage.waitForPageReady();

			// Toggle article content - should handle error gracefully
			await swipePage.toggleArticleContent();

			// Should not crash the page
			await expect(swipePage.swipeCard).toBeVisible();
		});
	});
});

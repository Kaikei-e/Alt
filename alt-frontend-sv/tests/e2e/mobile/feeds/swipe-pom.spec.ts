/**
 * Mobile Swipe Feed E2E Tests (Page Object Model)
 *
 * Tests for the swipe feed interface using Page Object Model pattern.
 * This demonstrates best practices for maintainable E2E tests.
 */

import {
	CONNECT_ARTICLE_CONTENT_RESPONSE,
	CONNECT_MARK_AS_READ_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { MobileSwipePage } from "../../pages/mobile/MobileSwipePage";
import { fulfillError, fulfillJson } from "../../utils/mockHelpers";

// Test data
const EMPTY_FEEDS = {
	data: [],
	nextCursor: "",
	hasMore: false,
};

test.describe("Mobile Swipe Feed - Page Object Model Tests", () => {
	let swipePage: MobileSwipePage;

	test.beforeEach(async ({ page }) => {
		swipePage = new MobileSwipePage(page);

		// Setup default mocks (+page.server.ts provides the 2 SSR cards "AI Trends" and "Svelte 5 Tips")
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, EMPTY_FEEDS),
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

			// Web-first assertion (auto-retrying) instead of a one-shot
			// textContent() read wrapped in expect(...).toContain(...).
			await expect(swipePage.swipeCard.locator("h2")).toContainText(
				"AI Trends",
			);
		});

		test("displays Pass and Read today buttons under the card", async () => {
			await swipePage.goto();
			await swipePage.waitForPageReady();

			await expect(swipePage.actionFooter).toBeVisible();
			await expect(swipePage.passButton).toBeVisible();
			await expect(swipePage.queueButton).toBeVisible();
		});

		test("external link has correct URL", async () => {
			await swipePage.goto();
			await swipePage.waitForPageReady();

			const link = swipePage.getExternalLink();
			await expect(link).toHaveAttribute(
				"href",
				"https://example.com/ai-trends",
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

			await expect(swipePage.passButton).toHaveAccessibleName("Pass");
			await expect(swipePage.queueButton).toHaveAccessibleName("Read today");
		});
	});

	test.describe("swipe interactions", () => {
		// Mark-as-read is deferred by one card so the header undo can cancel
		// it (dispatch session). The commit fires on the NEXT dismissal, or
		// immediately when the pile is exhausted.
		test("swipe left marks article as read", async ({ page }) => {
			let markAsReadCalled = false;
			await page.route(CONNECT_RPC_PATHS.markAsRead, (route) => {
				markAsReadCalled = true;
				return fulfillJson(route, CONNECT_MARK_AS_READ_RESPONSE);
			});

			await swipePage.goto();
			await swipePage.waitForPageReady();

			await swipePage.swipeLeft();
			// First dismissal is held in the undo window, not committed yet
			await expect(swipePage.swipeCard.locator("h2")).toContainText(
				"Svelte 5 Tips",
			);
			expect(markAsReadCalled).toBe(false);

			// Second dismissal commits the held card
			await swipePage.swipeLeft();

			await expect.poll(() => markAsReadCalled, { timeout: 5000 }).toBe(true);
		});

		test("swipe right queues article for Read today without marking read", async ({
			page,
		}) => {
			let markAsReadCalled = false;
			let queueArticleCalled = false;
			await page.route(CONNECT_RPC_PATHS.markAsRead, (route) => {
				markAsReadCalled = true;
				return fulfillJson(route, CONNECT_MARK_AS_READ_RESPONSE);
			});
			await page.route(CONNECT_RPC_PATHS.getReadTodayQueue, (route) =>
				fulfillJson(route, { items: [], cap: 7, readCountSince: 0 }),
			);
			await page.route(CONNECT_RPC_PATHS.queueArticle, (route) => {
				queueArticleCalled = true;
				return fulfillJson(route, {
					outcome: "QUEUE_ARTICLE_OUTCOME_QUEUED",
					items: [],
					cap: 7,
				});
			});

			await swipePage.goto();
			await swipePage.waitForPageReady();

			await swipePage.swipeRight();
			await expect(swipePage.swipeCard.locator("h2")).toContainText(
				"Svelte 5 Tips",
			);
			expect(queueArticleCalled).toBe(false);
			expect(markAsReadCalled).toBe(false);

			await swipePage.swipeRight();

			await expect.poll(() => queueArticleCalled, { timeout: 5000 }).toBe(true);
			expect(markAsReadCalled).toBe(false);
		});
	});

	test.describe("button interactions", () => {
		test("tapping card opens reader overlay and Back restores the card", async ({
			page,
		}) => {
			await swipePage.goto();
			await swipePage.waitForPageReady();

			await swipePage.toggleArticleContent();

			const overlay = page.getByTestId("reader-overlay");
			await expect(overlay).toBeVisible();

			await page.getByTestId("reader-overlay-close").click();
			await expect(overlay).toBeHidden();
			await expect(swipePage.swipeCard).toBeVisible();
		});
	});

	test.describe("empty state", () => {
		test("handles empty feed list gracefully", async ({ page }) => {
			await swipePage.goto();
			await swipePage.waitForPageReady();

			// Exhaust both SSR cards while cursor pagination returns EMPTY_FEEDS.
			await swipePage.swipeLeft();
			await expect(swipePage.swipeCard.locator("h2")).toContainText(
				"Svelte 5 Tips",
			);
			await swipePage.swipeLeft();

			// SwipeFeedScreen renders its "No more feeds" empty state when
			// there is no active feed and no error.
			await expect(page.getByText("No more feeds")).toBeVisible();
			await expect(page.getByRole("button", { name: "Refresh" })).toBeVisible();
			await expect(swipePage.swipeCard).not.toBeVisible();
		});
	});

	test.describe("error handling", () => {
		test("handles API error gracefully", async ({ page }) => {
			await swipePage.goto();
			await swipePage.waitForPageReady();

			await swipePage.swipeLeft();
			await expect(swipePage.swipeCard.locator("h2")).toContainText(
				"Svelte 5 Tips",
			);
			await swipePage.swipeLeft();
			await expect(page.getByText("No more feeds")).toBeVisible();

			await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
				fulfillError(route, "Internal Server Error", 500),
			);
			await page.getByRole("button", { name: "Refresh" }).click();

			// SwipeFeedScreen surfaces a distinct error state (not the
			// generic empty state) with a retry action.
			await expect(page.getByText("Error loading feeds")).toBeVisible();
			await expect(page.getByRole("button", { name: "Retry" })).toBeVisible();
			await expect(swipePage.swipeCard).not.toBeVisible();
		});

		test("handles article content fetch error", async ({ page }) => {
			await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
				fulfillError(route, "Not Found", 404),
			);

			await swipePage.goto();
			await swipePage.waitForPageReady();

			// Card 1 ("AI Trends") is SSR-seeded in articlePrefetcher; swipe to
			// Card 2 ("Svelte 5 Tips") which fetches article content on open.
			await swipePage.swipeLeft();
			await expect(swipePage.swipeCard.locator("h2")).toContainText(
				"Svelte 5 Tips",
			);

			await swipePage.toggleArticleContent();

			const overlay = page.getByTestId("reader-overlay");
			await expect(overlay).toBeVisible();
			await expect(overlay.getByTestId("article-content-failed")).toBeVisible();
		});
	});
});

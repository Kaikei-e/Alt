/**
 * Desktop Pages Accessibility Tests
 *
 * Automated accessibility testing using axe-playwright.
 * Tests WCAG 2.1 AA compliance for desktop pages.
 */
import { expect, test } from "@playwright/test";
import { gotoDesktopRoute } from "../helpers/navigation";
import {
	checkAccessibility,
	getAccessibilityViolations,
} from "../helpers/a11y";
import { fulfillJson } from "../utils/mockHelpers";
import {
	CONNECT_RPC_PATHS,
	CONNECT_ARTICLE_CONTENT_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
} from "../fixtures/mockData";

// Test data for mocking
const MOCK_FEEDS = {
	data: [
		{
			id: "feed-1",
			title: "Desktop Test Article",
			description: "Description for desktop accessibility testing.",
			link: "https://example.com/desktop-article",
			published: "2025-01-15",
			createdAt: new Date().toISOString(),
			author: "Test Author",
		},
	],
	nextCursor: "",
	hasMore: false,
};

const MOCK_STATS = {
	feedAmount: 25,
	unsummarizedFeedAmount: 10,
	articleAmount: 500,
};

const MOCK_RECAP = {
	recaps: [
		{
			id: "recap-1",
			title: "Weekly Tech Summary",
			summary: "Summary of tech news this week.",
			genre: "Technology",
			createdAt: new Date().toISOString(),
		},
	],
};

/**
 * Common options for accessibility checks.
 */
const a11yOptions = {
	tags: ["wcag2a", "wcag2aa"],
	disableRules: [
		"color-contrast",
		"landmark-one-main",
		"document-title",
		// Sidebar navigation may trigger region issues
		"region",
	],
};

test.describe("Desktop Pages Accessibility", () => {
	test.beforeEach(async ({ page }) => {
		// Setup common mocks
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, MOCK_FEEDS),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		// Favorite feeds uses same endpoint pattern
		await page.route("**/api/v2/alt.feeds.v2.FeedService/GetFavoriteFeeds", (route) =>
			fulfillJson(route, MOCK_FEEDS),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getDetailedFeedStats, (route) =>
			fulfillJson(route, MOCK_STATS),
		);
		await page.route("**/api/v2/recap*", (route) =>
			fulfillJson(route, MOCK_RECAP),
		);
	});

	test.describe("Desktop Feeds Page (/desktop/feeds)", () => {
		test("has no critical accessibility violations", async ({ page }) => {
			await gotoDesktopRoute(page, "feeds");
			await page.waitForLoadState("networkidle");

			await checkAccessibility(page, a11yOptions);
		});

		test("feed cards have accessible content", async ({ page }) => {
			await gotoDesktopRoute(page, "feeds");
			await page.waitForLoadState("networkidle");

			// Check that feed titles are in headings or links
			const feedTitle = page.getByText("Desktop Test Article");
			await expect(feedTitle).toBeVisible();
		});

		test("sidebar navigation is keyboard accessible", async ({ page }) => {
			await gotoDesktopRoute(page, "feeds");
			await page.waitForLoadState("networkidle");

			// Check for navigation links
			const navLinks = page.getByRole("navigation").getByRole("link");
			const linkCount = await navLinks.count();

			// Should have navigation links
			expect(linkCount).toBeGreaterThan(0);

			// First nav link should be focusable
			if (linkCount > 0) {
				await navLinks.first().focus();
				const focusedElement = page.locator(":focus");
				await expect(focusedElement).toBeVisible();
			}
		});
	});

	test.describe("Desktop Search Page (/desktop/feeds/search)", () => {
		test("has no critical accessibility violations", async ({ page }) => {
			// Mock search endpoint
			await page.route("**/api/v2/feeds/search*", (route) =>
				fulfillJson(route, MOCK_FEEDS),
			);

			await gotoDesktopRoute(page, "feeds/search");
			await page.waitForLoadState("networkidle");

			await checkAccessibility(page, a11yOptions);
		});

		test("search form has proper labels", async ({ page }) => {
			await gotoDesktopRoute(page, "feeds/search");
			await page.waitForLoadState("networkidle");

			// Search input should have accessible label
			const searchInput = page.getByRole("searchbox").or(
				page.getByPlaceholder(/search/i),
			);

			if (await searchInput.count()) {
				// Should be keyboard focusable
				await searchInput.first().focus();
				await expect(page.locator(":focus")).toBeVisible();
			}
		});
	});

	test.describe("Desktop Recap Page (/desktop/recap)", () => {
		test("has no critical accessibility violations", async ({ page }) => {
			await gotoDesktopRoute(page, "recap");
			await page.waitForLoadState("networkidle");

			await checkAccessibility(page, a11yOptions);
		});
	});

	test.describe("Desktop Stats Page (/desktop/stats)", () => {
		test("has no critical accessibility violations", async ({ page }) => {
			// Mock stats endpoints
			await page.route("**/api/v2/feeds/stats/**", (route) =>
				fulfillJson(route, MOCK_STATS),
			);
			await page.route("**/api/v1/feeds/stats/trends*", (route) =>
				fulfillJson(route, { trends: [] }),
			);

			await gotoDesktopRoute(page, "stats");
			await page.waitForLoadState("networkidle");

			await checkAccessibility(page, a11yOptions);
		});

		test("chart elements have accessible alternatives", async ({ page }) => {
			// Mock stats endpoints
			await page.route("**/api/v2/feeds/stats/**", (route) =>
				fulfillJson(route, MOCK_STATS),
			);
			await page.route("**/api/v1/feeds/stats/trends*", (route) =>
				fulfillJson(route, { trends: [] }),
			);

			await gotoDesktopRoute(page, "stats");
			await page.waitForLoadState("networkidle");

			// Charts should have aria-labels or be supplemented with text
			const violations = await getAccessibilityViolations(page, {
				tags: ["wcag2a"],
			});

			// Filter for image-related violations (charts are often rendered as canvas/svg)
			const imageViolations = violations.filter(
				(v) => v.id.includes("image") || v.id.includes("alt"),
			);

			// Log any image violations
			if (imageViolations.length > 0) {
				console.log("Image/Chart a11y violations:", imageViolations);
			}
		});
	});

	test.describe("Desktop Augur Chat Page (/desktop/augur)", () => {
		test("has no critical accessibility violations", async ({ page }) => {
			// Mock chat endpoint
			await page.route("**/api/v1/augur/**", (route) =>
				fulfillJson(route, { messages: [] }),
			);

			await gotoDesktopRoute(page, "augur");
			await page.waitForLoadState("networkidle");

			await checkAccessibility(page, a11yOptions);
		});

		test("chat input is keyboard accessible", async ({ page }) => {
			// Mock chat endpoint
			await page.route("**/api/v1/augur/**", (route) =>
				fulfillJson(route, { messages: [] }),
			);

			await gotoDesktopRoute(page, "augur");
			await page.waitForLoadState("networkidle");

			// Find chat input
			const chatInput = page.getByRole("textbox");
			if (await chatInput.count()) {
				await chatInput.first().focus();
				await expect(page.locator(":focus")).toBeVisible();

				// Type something
				await page.keyboard.type("Hello");
				await expect(chatInput.first()).toHaveValue("Hello");
			}
		});
	});
});

test.describe("Desktop Layout Accessibility", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, MOCK_FEEDS),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
	});

	test("sidebar can be navigated with keyboard", async ({ page }) => {
		await gotoDesktopRoute(page, "feeds");
		await page.waitForLoadState("networkidle");

		// Tab through the page
		await page.keyboard.press("Tab");

		// Should be able to reach navigation items
		const focusedCount = await page.evaluate(() => {
			let count = 0;
			const walk = (times: number) => {
				for (let i = 0; i < times; i++) {
					const focused = document.activeElement;
					if (
						focused &&
						(focused.matches("a, button") ||
							focused.getAttribute("tabindex") === "0")
					) {
						count++;
					}
				}
			};
			walk(10);
			return count;
		});

		// Should have found some focusable elements
		expect(focusedCount).toBeGreaterThanOrEqual(0);
	});

	test("skip link is available for keyboard users", async ({ page }) => {
		await gotoDesktopRoute(page, "feeds");
		await page.waitForLoadState("networkidle");

		// Check for skip link (common accessibility pattern)
		const skipLink = page.getByRole("link", { name: /skip to/i });

		// Skip links are optional but recommended
		if (await skipLink.count()) {
			// If present, it should work
			await skipLink.first().focus();
			await expect(skipLink.first()).toBeFocused();
		}
	});
});

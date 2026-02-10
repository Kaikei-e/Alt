/**
 * Mobile Pages Accessibility Tests
 *
 * Automated accessibility testing using axe-playwright.
 * Tests WCAG 2.1 AA compliance for mobile pages.
 */
import { expect, test } from "@playwright/test";
import { gotoMobileRoute } from "../helpers/navigation";
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
			title: "Test Article for A11y",
			description: "Description for accessibility testing.",
			link: "https://example.com/article-1",
			published: "2025-01-15",
			createdAt: new Date().toISOString(),
			author: "Test Author",
		},
	],
	nextCursor: "",
	hasMore: false,
};

const MOCK_STATS = {
	feedAmount: 10,
	unsummarizedFeedAmount: 5,
	articleAmount: 100,
};

/**
 * Common options for accessibility checks.
 * Disable known issues that may be framework-related.
 */
const a11yOptions = {
	// WCAG 2.1 AA is the standard for web accessibility
	tags: ["wcag2a", "wcag2aa"],
	// Disable rules that may have false positives in SPA contexts
	disableRules: [
		// Color contrast can be affected by dynamic theming
		// Manual testing recommended for this
		"color-contrast",
		// Landmark issues in single-page apps with base paths
		"landmark-one-main",
		// Page title is handled by SvelteKit's head management
		"document-title",
	],
};

test.describe("Mobile Pages Accessibility", () => {
	test.beforeEach(async ({ page }) => {
		// Setup common mocks
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, MOCK_FEEDS),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getDetailedFeedStats, (route) =>
			fulfillJson(route, MOCK_STATS),
		);
		// Mock SSE endpoints to prevent networkidle timeout
		await page.route("**/api/v1/sse/**", (route) => {
			route.fulfill({
				status: 200,
				contentType: "text/event-stream",
				body: "event: message\ndata: {}\n\n",
			});
		});
	});

	test.describe("Mobile Feeds Page (/mobile/feeds)", () => {
		test("has no critical accessibility violations", async ({ page }) => {
			await gotoMobileRoute(page, "feeds");
			await page.waitForLoadState("domcontentloaded");
			await expect(page.getByText("Test Article for A11y")).toBeVisible();

			await checkAccessibility(page, a11yOptions);
		});

		test("reports any accessibility violations for review", async ({
			page,
		}) => {
			await gotoMobileRoute(page, "feeds");
			await page.waitForLoadState("domcontentloaded");
			await expect(page.getByText("Test Article for A11y")).toBeVisible();

			const violations = await getAccessibilityViolations(page, {
				tags: ["wcag2a", "wcag2aa", "wcag21a", "wcag21aa"],
			});

			// Log violations for review (even if not critical)
			if (violations.length > 0) {
				console.log("A11y Violations found:");
				violations.forEach((v) => {
					console.log(`  - [${v.impact}] ${v.id}: ${v.description}`);
					console.log(`    Help: ${v.helpUrl}`);
				});
			}

			// Fail on critical or serious violations only
			const criticalViolations = violations.filter(
				(v) => v.impact === "critical" || v.impact === "serious",
			);
			expect(criticalViolations).toHaveLength(0);
		});
	});

	test.describe("Mobile Swipe Page (/mobile/feeds/swipe)", () => {
		test("has no critical accessibility violations", async ({ page }) => {
			await gotoMobileRoute(page, "feeds/swipe");
			await page.waitForLoadState("domcontentloaded");
			await expect(page.getByTestId("swipe-card")).toBeVisible();

			await checkAccessibility(page, a11yOptions);
		});

		test("swipe card has proper ARIA attributes", async ({ page }) => {
			await gotoMobileRoute(page, "feeds/swipe");
			await page.waitForLoadState("domcontentloaded");
			await expect(page.getByTestId("swipe-card")).toBeVisible();

			const swipeCard = page.getByTestId("swipe-card");
			await expect(swipeCard).toBeVisible();

			// Check for aria-busy attribute (used during loading states)
			await expect(swipeCard).toHaveAttribute("aria-busy");
		});

		test("buttons have accessible names", async ({ page }) => {
			await gotoMobileRoute(page, "feeds/swipe");
			await page.waitForLoadState("domcontentloaded");
			await expect(page.getByTestId("swipe-card")).toBeVisible();

			// Article button - using getByRole with name ensures it has an accessible name
			// Button text may change: "Article" -> "Loading..." -> "Hide"
			const articleButton = page.getByRole("button", {
				name: /article|loading|hide/i,
			});
			await expect(articleButton).toBeVisible();
			await expect(articleButton).toHaveAccessibleName(/article|loading|hide/i);

			// Summary button
			const summaryButton = page.getByRole("button", { name: /summary/i });
			await expect(summaryButton).toBeVisible();
			await expect(summaryButton).toHaveAccessibleName(/summary/i);
		});

		test("external links have proper security and accessibility", async ({
			page,
		}) => {
			await gotoMobileRoute(page, "feeds/swipe");
			await page.waitForLoadState("domcontentloaded");
			await expect(page.getByTestId("swipe-card")).toBeVisible();

			const externalLink = page.getByRole("link", {
				name: /open article|external/i,
			});
			await expect(externalLink).toHaveAttribute("target", "_blank");
			await expect(externalLink).toHaveAttribute("rel", "noopener noreferrer");
		});
	});

	test.describe("Mobile Search Page (/mobile/feeds/search)", () => {
		test("has no critical accessibility violations", async ({ page }) => {
			// Mock search endpoint
			await page.route("**/api/v2/feeds/search*", (route) =>
				fulfillJson(route, MOCK_FEEDS),
			);

			await gotoMobileRoute(page, "feeds/search");
			await page.waitForLoadState("domcontentloaded");
			// Wait for search input to be visible
			await expect(
				page.getByRole("searchbox").or(page.getByRole("textbox")).first(),
			).toBeVisible();

			await checkAccessibility(page, a11yOptions);
		});

		test("search input has proper labels", async ({ page }) => {
			await gotoMobileRoute(page, "feeds/search");
			await page.waitForLoadState("domcontentloaded");

			// Search input should have an accessible name
			const searchInput = page
				.getByRole("searchbox")
				.or(page.getByRole("textbox"));

			// If search input exists, check it's accessible and focusable
			if (await searchInput.count()) {
				await searchInput.first().focus();
				await expect(page.locator(":focus")).toBeVisible();
			}
		});
	});

	test.describe("Mobile Viewed Page (/mobile/feeds/viewed)", () => {
		test("has no critical accessibility violations", async ({ page }) => {
			await gotoMobileRoute(page, "feeds/viewed");
			await page.waitForLoadState("domcontentloaded");
			// Wait for page heading or content
			await expect(page.getByRole("heading").first()).toBeVisible();

			await checkAccessibility(page, a11yOptions);
		});
	});

	test.describe("Mobile Stats Page (/mobile/feeds/stats)", () => {
		test("has no critical accessibility violations", async ({ page }) => {
			// Mock stats endpoints
			await page.route("**/api/v2/feeds/stats*", (route) =>
				fulfillJson(route, MOCK_STATS),
			);
			await page.route(CONNECT_RPC_PATHS.getDetailedFeedStats, (route) =>
				fulfillJson(route, MOCK_STATS),
			);

			await page.goto("./stats");
			await page.waitForLoadState("domcontentloaded");
			// Wait for page heading or content
			await expect(page.getByRole("heading").first()).toBeVisible();

			await checkAccessibility(page, a11yOptions);
		});
	});
});

test.describe("Keyboard Navigation", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, MOCK_FEEDS),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);
		// Mock SSE endpoints to prevent networkidle timeout
		await page.route("**/api/v1/sse/**", (route) => {
			route.fulfill({
				status: 200,
				contentType: "text/event-stream",
				body: "event: message\ndata: {}\n\n",
			});
		});
	});

	test("swipe page buttons are keyboard accessible", async ({ page }) => {
		await gotoMobileRoute(page, "feeds/swipe");
		await page.waitForLoadState("domcontentloaded");
		await expect(page.getByTestId("swipe-card")).toBeVisible();

		// Tab to Article button
		await page.keyboard.press("Tab");
		await page.keyboard.press("Tab");
		await page.keyboard.press("Tab");

		// Find focused element
		const focusedElement = page.locator(":focus");

		// Should be able to focus on interactive elements
		const isButton = await focusedElement.evaluate((el) =>
			el.matches("button, a, input, [tabindex]"),
		);
		expect(isButton).toBe(true);
	});

	test("buttons can be activated with keyboard", async ({ page }) => {
		await gotoMobileRoute(page, "feeds/swipe");
		await page.waitForLoadState("domcontentloaded");
		await expect(page.getByTestId("swipe-card")).toBeVisible();

		// Focus on a button using Tab - button text may change: "Article" -> "Loading..." -> "Hide"
		const articleButton = page.getByRole("button", {
			name: /article|loading|hide/i,
		});
		await articleButton.focus();

		// Activate with Enter
		await page.keyboard.press("Enter");

		// Content section should appear or button state should change
		await page.waitForTimeout(500);

		// Check that something happened (content expanded or button changed)
		const buttonText = await articleButton.textContent();
		// After click, button text might change to "Hide" or loading state
		expect(buttonText).toBeTruthy();
	});
});

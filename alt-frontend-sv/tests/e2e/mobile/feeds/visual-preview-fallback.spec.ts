import { expect, test, type Locator } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson, fulfillError } from "../../utils/mockHelpers";
import {
	CONNECT_RPC_PATHS,
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
} from "../../fixtures/mockData";

const SUBSCRIPTIONS_EMPTY = { subscriptions: [] };
const BATCH_IMAGES_EMPTY = { results: [] };

const VISUAL_PREVIEW_PATHS = {
	listSubscriptions: "**/api/v2/alt.feeds.v2.FeedService/ListSubscriptions",
	batchPrefetchImages:
		"**/api/v2/alt.articles.v2.ArticleService/BatchPrefetchImages",
};

/**
 * Dispatch touch events to simulate a swipe left gesture.
 * Playwright's mouse events don't work for touch-based swipe detection.
 * See: https://playwright.dev/docs/touch-events
 */
async function swipeLeft(locator: Locator, distance: number = 150) {
	const { centerX, centerY } = await locator.evaluate((el: HTMLElement) => {
		const rect = el.getBoundingClientRect();
		return {
			centerX: rect.left + rect.width / 2,
			centerY: rect.top + rect.height / 2,
		};
	});

	// touchstart
	const startTouches = [{ identifier: 0, clientX: centerX, clientY: centerY }];
	await locator.dispatchEvent("touchstart", {
		touches: startTouches,
		changedTouches: startTouches,
		targetTouches: startTouches,
	});

	// touchmove in steps (swipe left = negative X)
	const steps = 5;
	for (let i = 1; i <= steps; i++) {
		const moveTouches = [{
			identifier: 0,
			clientX: centerX - (distance * i / steps),
			centerY,
		}];
		await locator.dispatchEvent("touchmove", {
			touches: moveTouches,
			changedTouches: moveTouches,
			targetTouches: moveTouches,
		});
	}

	// touchend
	const endTouches = [{ identifier: 0, clientX: centerX - distance, clientY: centerY }];
	await locator.dispatchEvent("touchend", {
		touches: [],
		changedTouches: endTouches,
		targetTouches: [],
	});
}

test.describe("mobile feeds — visual-preview 429 fallback", () => {
	test("surfaces source-unavailable notice when FetchArticleContent returns 429", async ({
		page,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(VISUAL_PREVIEW_PATHS.listSubscriptions, (route) =>
			fulfillJson(route, SUBSCRIPTIONS_EMPTY),
		);
		await page.route(VISUAL_PREVIEW_PATHS.batchPrefetchImages, (route) =>
			fulfillJson(route, BATCH_IMAGES_EMPTY),
		);
		// All FetchArticleContent calls return 429 (rate-limited),
		// reproducing the production symptom in ADR-000884.
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillError(route, "rate limit exceeded", 429),
		);

		await gotoMobileRoute(page, "feeds/swipe/visual-preview");

		const card = page.getByTestId("visual-preview-card").first();
		await expect(card).toBeVisible();

		// The first card has SSR-provided content from +page.server.ts (which hits
		// the mock backend, not Playwright's route mocks). Swipe left to dismiss
		// and move to the second card where client-side 429 handling applies.
		await swipeLeft(card, 200);

		// Wait for the swipe animation and card transition
		await page.waitForTimeout(600);

		const secondCard = page.getByTestId("visual-preview-card").first();
		await expect(secondCard).toBeVisible();

		// Description fallback (above-the-fold) must always render
		// so the card is never blank, even with auto-fetch failure.
		await expect(secondCard.getByText("Deep dive into the ecosystem.")).toBeVisible();

		// Tap "Article" to expand. Under 429, the in-card path must surface
		// the unified source-unavailable notice instead of leaving the
		// expanded section blank.
		await secondCard.getByRole("button", { name: /article/i }).click();

		await expect(secondCard.getByTestId("source-unavailable-notice")).toBeVisible();
		await expect(secondCard.getByTestId("article-fallback-summary")).toBeVisible();
	});
});

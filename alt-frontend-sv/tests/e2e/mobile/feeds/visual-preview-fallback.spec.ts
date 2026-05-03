import { expect, test } from "@playwright/test";
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
		const cardBox = await card.boundingBox();
		if (!cardBox) throw new Error("Card not visible");

		// Perform swipe left gesture
		await page.mouse.move(cardBox.x + cardBox.width * 0.8, cardBox.y + cardBox.height / 2);
		await page.mouse.down();
		await page.mouse.move(cardBox.x + cardBox.width * 0.1, cardBox.y + cardBox.height / 2, { steps: 10 });
		await page.mouse.up();

		// Wait for the transition to complete and second card to appear
		await page.waitForTimeout(500);

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

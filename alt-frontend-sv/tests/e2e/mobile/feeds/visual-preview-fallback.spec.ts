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

		// Description fallback (above-the-fold) must always render
		// so the card is never blank, even with auto-fetch failure.
		await expect(card.getByText("Deep dive into the ecosystem.")).toBeVisible();

		// Tap "Article" to expand. Under 429, the in-card path must surface
		// the unified source-unavailable notice instead of leaving the
		// expanded section blank.
		await card.getByRole("button", { name: /article/i }).click();

		await expect(card.getByTestId("source-unavailable-notice")).toBeVisible();
		await expect(card.getByTestId("article-fallback-summary")).toBeVisible();
	});
});

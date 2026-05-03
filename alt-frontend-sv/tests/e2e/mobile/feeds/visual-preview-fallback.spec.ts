import { expect, test } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_RPC_PATHS,
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
} from "../../fixtures/mockData";

const SUBSCRIPTIONS_EMPTY = { subscriptions: [] };

const VISUAL_PREVIEW_PATHS = {
	listSubscriptions: "**/api/v2/alt.feeds.v2.FeedService/ListSubscriptions",
};

test.describe("mobile feeds — network error fallback", () => {
	test("surfaces source-unavailable notice when FetchArticleContent fails", async ({
		page,
	}) => {
		// Use /feeds/swipe (ssr=false) instead of /feeds/swipe/visual-preview
		// because visual-preview has SSR that bypasses Playwright route mocks.
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(VISUAL_PREVIEW_PATHS.listSubscriptions, (route) =>
			fulfillJson(route, SUBSCRIPTIONS_EMPTY),
		);
		// Abort all FetchArticleContent calls to simulate network failure.
		// This reliably triggers the error fallback UI without depending on
		// Connect-RPC's specific error format handling.
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			route.abort("failed"),
		);

		// Use default swipe page which has ssr=false, so route mocks work
		await gotoMobileRoute(page, "feeds/swipe");

		const card = page.getByTestId("swipe-card").first();
		await expect(card).toBeVisible();

		// Description fallback (above-the-fold) must always render
		// so the card is never blank, even with auto-fetch failure.
		await expect(card.getByText("Deep dive into the ecosystem.")).toBeVisible();

		// Tap "Article" to expand. On network failure, the in-card path must
		// surface the source-unavailable notice with fallback summary.
		const articleButton = card.getByRole("button", { name: /article/i });
		await expect(articleButton).toBeVisible();
		await articleButton.click();

		// Wait for content section to expand and error state to render
		await expect(card.getByTestId("source-unavailable-notice")).toBeVisible({
			timeout: 10000,
		});
		await expect(card.getByTestId("article-fallback-summary")).toBeVisible();
	});
});

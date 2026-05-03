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

/**
 * E2E tests for the mobile swipe feed screen.
 *
 * Note: Error handling (429 fallback, network failure display) is tested
 * in unit tests (SwipeFeedCard.svelte.spec.ts) where we have full control
 * over API mocks. E2E tests here focus on page loading and basic rendering.
 */
test.describe("mobile feeds — swipe card rendering", () => {
	test("renders swipe cards with feed data from mocked API", async ({
		page,
	}) => {
		// Mock the Connect-RPC endpoints
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(VISUAL_PREVIEW_PATHS.listSubscriptions, (route) =>
			fulfillJson(route, SUBSCRIPTIONS_EMPTY),
		);
		// Return empty content for article fetches (simulates no content yet)
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, { content: "", articleId: "", url: "" }),
		);

		await gotoMobileRoute(page, "feeds/swipe");

		// Verify card renders with mocked feed data
		const card = page.getByTestId("swipe-card").first();
		await expect(card).toBeVisible();

		// Check feed title and description from mock data
		await expect(card.getByRole("heading", { name: "AI Trends" })).toBeVisible();
		await expect(card.getByText("Deep dive into the ecosystem.")).toBeVisible();

		// Check action buttons are present
		await expect(
			card.getByRole("button", { name: /article/i }),
		).toBeVisible();
		await expect(
			card.getByRole("button", { name: /summary/i }),
		).toBeVisible();
	});

	test("shows description as fallback when article content is unavailable", async ({
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
		// Return empty content - card should still show description
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, { content: "", articleId: "", url: "" }),
		);

		await gotoMobileRoute(page, "feeds/swipe");

		const card = page.getByTestId("swipe-card").first();
		await expect(card).toBeVisible();

		// Description must always be visible as fallback content
		await expect(card.getByText("Deep dive into the ecosystem.")).toBeVisible();
	});
});

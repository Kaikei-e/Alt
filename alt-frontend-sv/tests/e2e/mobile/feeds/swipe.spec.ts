import { expect, test } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_RPC_PATHS,
	CONNECT_ARTICLE_CONTENT_RESPONSE,
} from "../../fixtures/mockData";

// Swipe mode uses Connect-RPC for data fetching (SSR disabled)
const SWIPE_FEEDS_RESPONSE = {
	data: [
		{
			id: "feed-1",
			title: "AI Trends",
			description: "Latest AI updates across the ecosystem.",
			link: "https://example.com/ai-trends",
			published: "2 hours ago",
			createdAt: new Date().toISOString(),
			author: "Alice",
		},
	],
	nextCursor: "next-cursor-123",
	hasMore: true,
};

const VIEWED_FEEDS_EMPTY = {
	data: [],
	nextCursor: "",
	hasMore: false,
};

test.describe("mobile feeds routes - swipe", () => {
	test("swipe page renders swipe card and action footer", async ({ page }) => {
		// Mock Connect-RPC endpoints (used by +page.ts loader and client-side components)
		// Note: SSR is disabled for this page, all data is fetched client-side via Connect-RPC
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, SWIPE_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, VIEWED_FEEDS_EMPTY),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);

		await gotoMobileRoute(page, "feeds/swipe");

		await expect(page.getByTestId("swipe-card")).toBeVisible();
		await expect(
			page.getByRole("heading", { name: "AI Trends" }),
		).toBeVisible();
		await expect(page.getByTestId("action-footer")).toBeVisible();
	});
});

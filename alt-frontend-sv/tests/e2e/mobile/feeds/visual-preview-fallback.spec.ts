import {
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson } from "../../utils/mockHelpers";

const SUBSCRIPTIONS_EMPTY = { subscriptions: [] };

const VISUAL_PREVIEW_PATHS = {
	listSubscriptions: "**/api/v2/alt.feeds.v2.FeedService/ListSubscriptions",
};

test.describe("mobile feeds — swipe card rendering", () => {
	test("renders swipe cards with feed data from mocked API", async ({
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
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, { content: "", articleId: "", url: "" }),
		);

		await gotoMobileRoute(page, "feeds/swipe/visual-preview");

		const card = page.getByTestId("visual-preview-card").first();
		await expect(card).toBeVisible();

		await expect(
			card.getByRole("heading", { name: "AI Trends" }),
		).toBeVisible();
		await expect(card.getByText("Deep dive into the ecosystem.")).toBeVisible();
		await expect(
			card.getByRole("button", { name: /^article$/i }),
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
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, { content: "", articleId: "", url: "" }),
		);

		await gotoMobileRoute(page, "feeds/swipe/visual-preview");

		const card = page.getByTestId("visual-preview-card").first();
		await expect(card).toBeVisible();
		await expect(card.getByText("Deep dive into the ecosystem.")).toBeVisible();
	});
});

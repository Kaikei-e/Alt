import { expect, test } from "@playwright/test";
import { DesktopFeedsPage } from "../../pages/desktop/DesktopFeedsPage";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_ARTICLE_CONTENT_RESPONSE,
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";

test.describe("Desktop Feeds - StreamSummarize", () => {
	test("renders streamed summary chunks before the stream completes", async ({
		page,
	}) => {
		const feedsPage = new DesktopFeedsPage(page);

		await page.route(CONNECT_RPC_PATHS.getAllFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);

		await feedsPage.goto();
		await feedsPage.waitForFeedsLoaded();
		await feedsPage.selectFeed("AI Trends");

		await expect(
			page.getByRole("button", { name: /re-fetch article/i }),
		).toBeVisible({ timeout: 10_000 });

		await feedsPage.summarizeButton.click();

		await expect(page.getByText("First streamed sentence.")).toBeVisible({
			timeout: 10_000,
		});
		await expect(
			page.getByRole("button", { name: /summarizing/i }),
		).toBeVisible();

		await expect(
			page.getByText(
				"First streamed sentence. Second streamed sentence. Final streamed sentence.",
			),
		).toBeVisible({ timeout: 10_000 });
		await expect(
			page.getByRole("button", { name: /re-summarize/i }),
		).toBeVisible();
	});
});

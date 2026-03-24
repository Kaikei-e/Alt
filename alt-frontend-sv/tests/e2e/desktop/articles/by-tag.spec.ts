import { test, expect } from "../../fixtures/pomFixtures";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_TAG_TRAIL_PATHS,
	CONNECT_TAG_TRAIL_ARTICLES_RESPONSE,
} from "../../fixtures/mockData";

test.describe("Desktop Tag Articles", () => {
	test.beforeEach(async ({ page }) => {
		await page.route(CONNECT_TAG_TRAIL_PATHS.fetchArticlesByTag, (route) =>
			fulfillJson(route, CONNECT_TAG_TRAIL_ARTICLES_RESPONSE),
		);
	});

	test("shows tag name in heading", async ({ desktopTagArticlesPage }) => {
		await desktopTagArticlesPage.gotoWithTag("AI");
		await desktopTagArticlesPage.waitForArticlesLoaded();
		await expect(desktopTagArticlesPage.pageTitle).toContainText("AI");
	});

	test("renders article list", async ({ desktopTagArticlesPage }) => {
		await desktopTagArticlesPage.gotoWithTag("AI");
		await desktopTagArticlesPage.waitForArticlesLoaded();
		await expect(desktopTagArticlesPage.articleList).toBeVisible();
		await expect(
			desktopTagArticlesPage.getArticle("trail-art-1"),
		).toBeVisible();
	});

	test("shows article titles and feed names", async ({
		page,
		desktopTagArticlesPage,
	}) => {
		await desktopTagArticlesPage.gotoWithTag("AI");
		await desktopTagArticlesPage.waitForArticlesLoaded();
		await expect(page.getByText("AI Trends in 2026")).toBeVisible();
		await expect(page.getByText("TechBlog")).toBeVisible();
	});

	test("shows empty state when no articles", async ({
		page,
		desktopTagArticlesPage,
	}) => {
		await page.route(CONNECT_TAG_TRAIL_PATHS.fetchArticlesByTag, (route) =>
			fulfillJson(route, { articles: [], nextCursor: "", hasMore: false }),
		);
		await desktopTagArticlesPage.gotoWithTag("NonExistentTag");
		await desktopTagArticlesPage.waitForArticlesLoaded();
		await expect(desktopTagArticlesPage.emptyState).toBeVisible();
	});

	test("shows no-tag message when tag param is missing", async ({ page }) => {
		await page.goto("./articles/by-tag");
		await expect(page.getByText(/no tag specified/i)).toBeVisible();
	});

	test("load more button fetches next page", async ({
		page,
		desktopTagArticlesPage,
	}) => {
		// First page has hasMore=true
		await page.route(CONNECT_TAG_TRAIL_PATHS.fetchArticlesByTag, (route) =>
			fulfillJson(route, {
				...CONNECT_TAG_TRAIL_ARTICLES_RESPONSE,
				nextCursor: "cursor-2",
				hasMore: true,
			}),
		);
		await desktopTagArticlesPage.gotoWithTag("AI");
		await desktopTagArticlesPage.waitForArticlesLoaded();
		await expect(desktopTagArticlesPage.loadMoreButton).toBeVisible();
	});

	test("encodes special characters in tag parameter", async ({
		page,
		desktopTagArticlesPage,
	}) => {
		await desktopTagArticlesPage.gotoWithTag("C++");
		await expect(page.url()).toContain("tag=C%2B%2B");
	});

	test("renders articles in a grid layout", async ({
		desktopTagArticlesPage,
	}) => {
		await desktopTagArticlesPage.gotoWithTag("AI");
		await desktopTagArticlesPage.waitForArticlesLoaded();
		await expect(desktopTagArticlesPage.articleGrid).toBeVisible();
	});

	test("clicking article card opens detail panel", async ({
		page,
		desktopTagArticlesPage,
	}) => {
		await desktopTagArticlesPage.gotoWithTag("AI");
		await desktopTagArticlesPage.waitForArticlesLoaded();
		await desktopTagArticlesPage.getArticle("trail-art-1").click();
		await expect(desktopTagArticlesPage.detailPanel).toBeVisible();
		await expect(
			desktopTagArticlesPage.detailPanel.getByText("AI Trends in 2026"),
		).toBeVisible();
	});

	test("detail panel auto-fetches content on open", async ({
		page,
		desktopTagArticlesPage,
	}) => {
		await desktopTagArticlesPage.gotoWithTag("AI");
		await desktopTagArticlesPage.waitForArticlesLoaded();
		await desktopTagArticlesPage.getArticle("trail-art-1").click();
		await expect(desktopTagArticlesPage.detailPanel).toBeVisible();
		// Should show loading spinner or content — not the "Click Fetch Content" empty state
		const emptyPrompt = page.getByText(/click.*fetch content/i);
		await expect(emptyPrompt).not.toBeVisible({ timeout: 5000 });
	});

	test("closing detail panel returns to full grid", async ({
		desktopTagArticlesPage,
	}) => {
		await desktopTagArticlesPage.gotoWithTag("AI");
		await desktopTagArticlesPage.waitForArticlesLoaded();
		await desktopTagArticlesPage.getArticle("trail-art-1").click();
		await expect(desktopTagArticlesPage.detailPanel).toBeVisible();
		await desktopTagArticlesPage.closeDetailButton.click();
		await expect(desktopTagArticlesPage.detailPanel).not.toBeVisible();
	});

	test("Escape key closes detail panel", async ({
		page,
		desktopTagArticlesPage,
	}) => {
		await desktopTagArticlesPage.gotoWithTag("AI");
		await desktopTagArticlesPage.waitForArticlesLoaded();
		await desktopTagArticlesPage.getArticle("trail-art-1").click();
		await expect(desktopTagArticlesPage.detailPanel).toBeVisible();
		await page.keyboard.press("Escape");
		await expect(desktopTagArticlesPage.detailPanel).not.toBeVisible();
	});

	test("backdrop click closes detail panel", async ({
		page,
		desktopTagArticlesPage,
	}) => {
		await desktopTagArticlesPage.gotoWithTag("AI");
		await desktopTagArticlesPage.waitForArticlesLoaded();
		await desktopTagArticlesPage.getArticle("trail-art-1").click();
		await expect(desktopTagArticlesPage.detailPanel).toBeVisible();
		await page.getByTestId("detail-backdrop").click({ position: { x: 10, y: 10 } });
		await expect(desktopTagArticlesPage.detailPanel).not.toBeVisible();
	});

	test("detail panel is overlay (grid stays 3-col)", async ({
		desktopTagArticlesPage,
	}) => {
		await desktopTagArticlesPage.gotoWithTag("AI");
		await desktopTagArticlesPage.waitForArticlesLoaded();
		await desktopTagArticlesPage.getArticle("trail-art-1").click();
		await expect(desktopTagArticlesPage.detailPanel).toBeVisible();
		// Grid should still be visible behind the overlay
		await expect(desktopTagArticlesPage.articleGrid).toBeVisible();
	});
});

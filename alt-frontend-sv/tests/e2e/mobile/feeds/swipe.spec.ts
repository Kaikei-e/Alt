import {
	CONNECT_ARTICLE_CONTENT_RESPONSE,
	CONNECT_MARK_AS_READ_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillError, fulfillJson } from "../../utils/mockHelpers";

const SWIPE_FEEDS_RESPONSE = {
	data: [
		{
			id: "feed-1",
			articleId: "01938e82-7c00-7a7b-9b10-000000000001",
			title: "AI Trends",
			description: "Latest AI updates across the ecosystem.",
			link: "https://example.com/ai-trends",
			published: "2 hours ago",
			createdAt: new Date().toISOString(),
			author: "Alice",
		},
		{
			id: "feed-2",
			articleId: "01938e82-7c00-7a7b-9b10-000000000002",
			title: "Systems Engineering",
			description: "Notes on distributed systems.",
			link: "https://example.com/systems",
			published: "3 hours ago",
			createdAt: new Date().toISOString(),
			author: "Bob",
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
	test("swipe page renders visual-preview card with Pass/Read today buttons and no Keep/Article/Summary", async ({
		page,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, SWIPE_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, VIEWED_FEEDS_EMPTY),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);

		await gotoMobileRoute(page, "feeds/swipe/visual-preview");

		const card = page.getByTestId("visual-preview-card");
		await expect(card).toBeVisible();
		await expect(
			card.getByRole("heading", { name: "AI Trends" }),
		).toBeVisible();
		await expect(page.getByTestId("swipe-actions")).toBeVisible();
		await expect(page.getByRole("button", { name: "Pass" })).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Read today" }),
		).toBeVisible();
		await expect(card.getByRole("button", { name: /^article$/i })).toHaveCount(
			0,
		);
		await expect(card.getByTestId("keep-stamp")).toHaveCount(0);
		await expect(card.getByRole("button", { name: /summary/i })).toHaveCount(0);
	});

	test("tapping card opens reader overlay at /articles/<id> and Back restores the same top card", async ({
		page,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, SWIPE_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, VIEWED_FEEDS_EMPTY),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, {
				url: "https://example.com/ai-trends",
				content: "<p>Full AI Trends body from swipe.</p>",
				articleId: "01938e82-7c00-7a7b-9b10-000000000001",
			}),
		);

		let markAsReadCalled = false;
		await page.route(CONNECT_RPC_PATHS.markAsRead, (route) => {
			markAsReadCalled = true;
			return fulfillJson(route, CONNECT_MARK_AS_READ_RESPONSE);
		});

		await gotoMobileRoute(page, "feeds/swipe/visual-preview");

		const card = page.getByTestId("visual-preview-card");
		await expect(card).toBeVisible();
		await expect(
			card.getByRole("heading", { name: "AI Trends" }),
		).toBeVisible();

		await card.getByTestId("unified-scroll-area").click();

		const overlay = page.getByTestId("reader-overlay");
		await expect(overlay).toBeVisible();
		await expect(
			overlay.getByText("This is a mocked article content for E2E testing."),
		).toBeVisible();
		await expect
			.poll(() => new URL(page.url()).pathname)
			.toBe("/articles/01938e82-7c00-7a7b-9b10-000000000123");
		expect(markAsReadCalled).toBe(false);

		await page.goBack();
		await expect(overlay).toBeHidden();
		await expect
			.poll(() => new URL(page.url()).pathname)
			.toBe("/feeds/swipe/visual-preview");
		await expect(
			card.getByRole("heading", { name: "AI Trends" }),
		).toBeVisible();
		expect(markAsReadCalled).toBe(false);
	});

	test("swipe marks feed as read even without articleId (404 article)", async ({
		page,
	}) => {
		let unreadFetchCount = 0;
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) => {
			unreadFetchCount += 1;
			if (unreadFetchCount === 1) {
				return fulfillJson(route, {
					data: [
						{
							id: "feed-no-article",
							articleId: "",
							title: "Feed Without Article",
							description: "Unsaved article feed item.",
							link: "https://example.com/feed-without-article",
							published: "1 hour ago",
							createdAt: new Date().toISOString(),
							author: "Carol",
						},
					],
					nextCursor: null,
					hasMore: false,
				});
			}
			return fulfillJson(route, VIEWED_FEEDS_EMPTY);
		});
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillError(route, "Article not found", 404),
		);

		let markAsReadCalled = false;
		await page.route(CONNECT_RPC_PATHS.markAsRead, (route) => {
			markAsReadCalled = true;
			return fulfillJson(route, CONNECT_MARK_AS_READ_RESPONSE);
		});

		await gotoMobileRoute(page, "feeds/swipe/visual-preview");
		const card = page.getByTestId("visual-preview-card");
		await expect(card).toBeVisible();

		// SSR loads 2 cards ("AI Trends", "Svelte 5 Tips") and client loadMore
		// appends "Feed Without Article".
		const swipeLeftOnce = async () => {
			await expect(page.getByTestId("swipe-progress-indicator")).toBeHidden();
			const box = await card.boundingBox();
			if (!box) throw new Error("Card not found");
			await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
			await page.mouse.down();
			await page.mouse.move(box.x - 200, box.y + box.height / 2, { steps: 10 });
			await page.mouse.up();
		};

		await swipeLeftOnce();
		await expect(
			card.getByRole("heading", { name: "Svelte 5 Tips" }),
		).toBeVisible();

		await swipeLeftOnce();
		await expect(
			card.getByRole("heading", { name: "Feed Without Article" }),
		).toBeVisible();

		await swipeLeftOnce();

		await expect(page.getByText("No more feeds")).toBeVisible({
			timeout: 5000,
		});
		await expect.poll(() => markAsReadCalled, { timeout: 5000 }).toBe(true);
	});
});

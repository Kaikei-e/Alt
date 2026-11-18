import {
	CONNECT_ARTICLE_CONTENT_RESPONSE,
	CONNECT_MARK_AS_READ_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../fixtures/mockData";
import { expect, test } from "../fixtures/pomFixtures";
import { gotoMobileRoute } from "../helpers/navigation";
import { fulfillJson } from "../utils/mockHelpers";

function buildDeck(count: number) {
	return {
		data: Array.from({ length: count }, (_, i) => ({
			id: `deck-feed-${i + 1}`,
			feedId: `feed-src-${i + 1}`,
			articleId: `deck-art-${i + 1}`,
			title: `Deck Article ${i + 1}`,
			description: `Summary for deck article ${i + 1}.`,
			link: `https://example.com/deck-${i + 1}`,
			published: "1 hour ago",
			createdAt: new Date(Date.now() - i * 60_000).toISOString(),
			author: `Source ${i + 1}`,
		})),
		nextCursor: "",
		hasMore: false,
	};
}

async function swipeCard(
	page: import("@playwright/test").Page,
	direction: "left" | "right",
) {
	const card = page.getByTestId("visual-preview-card");
	await expect(card).toBeVisible();
	const box = await card.boundingBox();
	if (!box) throw new Error("Card bounding box missing");
	const cx = box.x + box.width / 2;
	const cy = box.y + box.height / 2;
	const targetX = direction === "right" ? box.x + box.width + 200 : box.x - 200;

	await page.mouse.move(cx, cy);
	await page.mouse.down();
	await page.mouse.move(targetX, cy, { steps: 10 });
	await page.mouse.up();
}

test.describe("Read today mobile flow", () => {
	test("/ lands on /today and bottom nav shows five tabs with Today active", async ({
		page,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getReadTodayQueue, (route) =>
			fulfillJson(route, { items: [], cap: 7, readCountSince: 0 }),
		);

		await page.goto("./");

		await expect.poll(() => new URL(page.url()).pathname).toBe("/today");
		await expect(
			page.getByRole("heading", { level: 1, name: "Today" }),
		).toBeVisible();

		const nav = page.getByRole("navigation", { name: "Main navigation" });
		await expect(nav).toBeVisible();

		const links = nav.getByRole("link");
		await expect(links).toHaveCount(5);
		const expectedTabs = [
			{ label: "Today", href: "/today" },
			{ label: "Swipe", href: "/feeds/swipe/visual-preview" },
			{ label: "Gallery", href: "/feeds/visual-preview" },
			{ label: "Search", href: "/search" },
			{ label: "Menu", href: "/menu" },
		];
		for (const [i, tab] of expectedTabs.entries()) {
			const link = links.nth(i);
			await expect(link).toContainText(tab.label);
			await expect(link).toHaveAttribute("href", tab.href);
		}
		await expect(links.nth(0)).toHaveAttribute("aria-current", "page");
		await expect(links.nth(0)).toHaveAttribute("data-active", "true");
		await expect(links.nth(1)).not.toHaveAttribute("aria-current");
	});

	test("right swipe queues via mock backend, item appears on /today, and Remove removes it", async ({
		page,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, { data: [], nextCursor: "", hasMore: false }),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, CONNECT_ARTICLE_CONTENT_RESPONSE),
		);

		let markAsReadCalls = 0;
		await page.route(CONNECT_RPC_PATHS.markAsRead, (route) => {
			markAsReadCalls++;
			return fulfillJson(route, CONNECT_MARK_AS_READ_RESPONSE);
		});

		await gotoMobileRoute(page, "feeds/swipe/visual-preview");
		const card = page.getByTestId("visual-preview-card");
		await expect(
			card.getByRole("heading", { name: "AI Trends" }),
		).toBeVisible();

		// Right swipe card 1 ("AI Trends"), then left swipe card 2 ("Svelte 5 Tips")
		// to flush card 1's queue action to the mock backend.
		await swipeCard(page, "right");
		await expect(
			card.getByRole("heading", { name: "Svelte 5 Tips" }),
		).toBeVisible();
		await swipeCard(page, "left");
		await expect(page.getByText("No more feeds")).toBeVisible();
		expect(markAsReadCalls).toBe(1);

		await gotoMobileRoute(page, "today");
		await expect(page.getByTestId("today-queue-count")).toContainText("1 / 7");
		await expect(page.getByText("AI Trends")).toBeVisible();

		await page.getByRole("button", { name: /Remove AI Trends/i }).click();
		await expect(page.getByText("Nothing queued.")).toBeVisible();
		await expect(page.getByTestId("today-queue-count")).toContainText("0 / 7");
	});

	test("left swipe passes, right swipe queues, and UNDO works for both", async ({
		page,
	}) => {
		const queuedIds: string[] = [];
		const passedLinks: string[] = [];

		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, buildDeck(3)),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadTodayQueue, (route) =>
			fulfillJson(route, { items: [], cap: 7, readCountSince: 0 }),
		);
		await page.route(CONNECT_RPC_PATHS.markAsRead, (route) => {
			const body = route.request().postDataJSON() as { articleUrl?: string };
			if (body?.articleUrl) passedLinks.push(body.articleUrl);
			return fulfillJson(route, CONNECT_MARK_AS_READ_RESPONSE);
		});
		await page.route(CONNECT_RPC_PATHS.queueArticle, (route) => {
			const body = route.request().postDataJSON() as { articleId?: string };
			if (body?.articleId) queuedIds.push(body.articleId);
			return fulfillJson(route, {
				outcome: "QUEUE_ARTICLE_OUTCOME_QUEUED",
				items: [],
				cap: 7,
			});
		});

		await gotoMobileRoute(page, "feeds/swipe/visual-preview");
		const card = page.getByTestId("visual-preview-card");
		await expect(
			card.getByRole("heading", { name: "AI Trends" }),
		).toBeVisible();

		// Left swipe then UNDO cancels pass
		await swipeCard(page, "left");
		await expect(
			card.getByRole("heading", { name: "Svelte 5 Tips" }),
		).toBeVisible();
		await expect(page.getByTestId("dispatch-counter")).toHaveText("№ 1");
		await page.getByTestId("dispatch-undo").click();
		await expect(
			card.getByRole("heading", { name: "AI Trends" }),
		).toBeVisible();
		await expect(page.getByTestId("dispatch-counter")).toHaveText("№ 0");
		expect(passedLinks).toHaveLength(0);

		// Right swipe then UNDO cancels queue
		await swipeCard(page, "right");
		await expect(
			card.getByRole("heading", { name: "Svelte 5 Tips" }),
		).toBeVisible();
		await page.getByTestId("dispatch-undo").click();
		await expect(
			card.getByRole("heading", { name: "AI Trends" }),
		).toBeVisible();
		expect(queuedIds).toHaveLength(0);

		// Left swipe card 1 -> passes when card 2 is swiped left
		await swipeCard(page, "left");
		await expect(
			card.getByRole("heading", { name: "Svelte 5 Tips" }),
		).toBeVisible();
		await swipeCard(page, "left");
		await expect.poll(() => passedLinks.length).toBe(1);
		expect(queuedIds).toHaveLength(0);
	});

	test("8th right swipe is refused with full notice", async ({ page }) => {
		const queuedItems: Array<{
			articleId: string;
			title: string;
			url: string;
			sourceName: string;
			imageUrl: string;
			queuedAt: string;
			expiresAt: string;
		}> = [];

		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, buildDeck(8)),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadTodayQueue, (route) =>
			fulfillJson(route, { items: queuedItems, cap: 7, readCountSince: 0 }),
		);
		await page.route(CONNECT_RPC_PATHS.queueArticle, (route) => {
			const body = route.request().postDataJSON() as { articleId?: string };
			if (queuedItems.length >= 7) {
				return fulfillJson(route, {
					outcome: "QUEUE_ARTICLE_OUTCOME_QUEUE_FULL",
					items: queuedItems,
					cap: 7,
				});
			}
			queuedItems.push({
				articleId: body.articleId ?? "art",
				title: `Title ${body.articleId}`,
				url: `https://example.com/${body.articleId}`,
				sourceName: "Source",
				imageUrl: "",
				queuedAt: new Date().toISOString(),
				expiresAt: new Date(Date.now() + 48 * 3600_000).toISOString(),
			});
			return fulfillJson(route, {
				outcome: "QUEUE_ARTICLE_OUTCOME_QUEUED",
				items: queuedItems,
				cap: 7,
			});
		});

		await gotoMobileRoute(page, "feeds/swipe/visual-preview");
		const card = page.getByTestId("visual-preview-card");

		for (let i = 0; i < 7; i++) {
			await expect(card).toBeVisible();
			await swipeCard(page, "right");
			await expect(page.getByTestId("dispatch-counter")).toHaveText(
				`№ ${i + 1}`,
			);
		}

		// 8th right swipe must be refused immediately because 6 committed + 1 pending = 7
		const eighthTitle = await card.locator("h2").textContent();
		await swipeCard(page, "right");

		await expect(page.getByTestId("swipe-notice")).toHaveText(
			"Read today is full (7). Read or remove one first.",
		);
		await expect(card.locator("h2")).toHaveText(eighthTitle ?? "");
	});

	test("Pass and Read today buttons mirror the swipes", async ({ page }) => {
		const queuedIds: string[] = [];
		const passedLinks: string[] = [];

		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, { data: [], nextCursor: "", hasMore: false }),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadTodayQueue, (route) =>
			fulfillJson(route, { items: [], cap: 7, readCountSince: 0 }),
		);
		await page.route(CONNECT_RPC_PATHS.markAsRead, (route) => {
			const body = route.request().postDataJSON() as { articleUrl?: string };
			if (body?.articleUrl) passedLinks.push(body.articleUrl);
			return fulfillJson(route, CONNECT_MARK_AS_READ_RESPONSE);
		});
		await page.route(CONNECT_RPC_PATHS.queueArticle, (route) => {
			const body = route.request().postDataJSON() as { articleId?: string };
			if (body?.articleId) queuedIds.push(body.articleId);
			return fulfillJson(route, {
				outcome: "QUEUE_ARTICLE_OUTCOME_QUEUED",
				items: [],
				cap: 7,
			});
		});

		await gotoMobileRoute(page, "feeds/swipe/visual-preview");
		const card = page.getByTestId("visual-preview-card");
		await expect(
			card.getByRole("heading", { name: "AI Trends" }),
		).toBeVisible();

		await page.getByTestId("swipe-queue-button").click();
		await expect(
			card.getByRole("heading", { name: "Svelte 5 Tips" }),
		).toBeVisible();
		await expect(page.getByTestId("dispatch-counter")).toHaveText("№ 1");

		await page.getByTestId("swipe-pass-button").click();
		await expect(page.getByText("No more feeds")).toBeVisible();

		await expect.poll(() => queuedIds).toEqual(["article-1"]);
		await expect
			.poll(() => passedLinks)
			.toEqual(["https://example.com/svelte-5"]);
	});
});

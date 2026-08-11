import { expect, test } from "../../fixtures/pomFixtures";
import {
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { DesktopFeedsPage } from "../../pages/desktop/DesktopFeedsPage";
import { fulfillJson } from "../../utils/mockHelpers";

/**
 * On-demand OG image resolution.
 *
 * Roughly three quarters of feed rows never carry an og:image in their RSS, and
 * most of those have no article row either — so neither the old server-side
 * crawl nor the article-keyed client prefetch could ever reach them. Resolution
 * is therefore keyed on the *feed*, and it happens when the reader actually
 * brings the card into view: one page fetch, for one item, at the moment
 * someone is looking at it.
 *
 * Contract this spec pins:
 *  - a card with no proxy URL asks for one when it scrolls into view;
 *  - the request is keyed on feed id and works with no articleId at all;
 *  - cards the reader never reaches are never requested;
 *  - an unresolvable feed settles on the fallback and is asked for exactly
 *    once — scrolling past it again must not re-scrape the origin.
 */

// 1x1 transparent PNG.
const PNG_1x1 = Buffer.from(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
	"base64",
);

/** Build an image-proxy URL whose base64 segment decodes to a real upstream host. */
function proxyUrl(original: string): string {
	return `/v1/images/proxy/testsig/${Buffer.from(original).toString("base64")}`;
}

function feedWithoutOg(id: string, title: string, slug: string) {
	return {
		id,
		// Deliberately empty: the 92% of image-less feeds that have no article
		// row are exactly the ones the old article-keyed prefetch skipped.
		articleId: "",
		title,
		description: "Deep dive into the ecosystem.",
		link: `https://example.com/${slug}`,
		published: "2 hours ago",
		createdAt: new Date().toISOString(),
		author: "Alice",
		ogImageProxyUrl: "",
	};
}

const FEEDS_WITHOUT_OG = {
	data: [
		feedWithoutOg("feed-1", "AI Trends", "ai-trends"),
		feedWithoutOg("feed-2", "Svelte 5 Tips", "svelte-5"),
	],
	nextCursor: "",
	hasMore: false,
};

async function stubFeedLists(page: import("@playwright/test").Page) {
	await page.route(CONNECT_RPC_PATHS.getAllFeeds, (r) =>
		fulfillJson(r, FEEDS_WITHOUT_OG),
	);
	await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (r) =>
		fulfillJson(r, FEEDS_WITHOUT_OG),
	);
	await page.route(CONNECT_RPC_PATHS.getReadFeeds, (r) =>
		fulfillJson(r, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
	);
}

test.describe("Visual Preview — on-demand OG image resolution", () => {
	test("a card with no proxy URL resolves one on viewport entry and shows the image", async ({
		page,
	}) => {
		await stubFeedLists(page);

		const resolveRequests: string[][] = [];
		await page.route(CONNECT_RPC_PATHS.resolveOgImages, async (route) => {
			const body = route.request().postDataJSON() as { feedIds?: string[] };
			const ids = body?.feedIds ?? [];
			resolveRequests.push(ids);
			await fulfillJson(route, {
				images: ids.map((feedId) => ({
					feedId,
					ogImageProxyUrl: proxyUrl(`https://img.example.com/${feedId}.png`),
				})),
			});
		});

		await page.route("**/v1/images/proxy/**", (route) =>
			route.fulfill({ status: 200, contentType: "image/png", body: PNG_1x1 }),
		);

		const feedsPage = new DesktopFeedsPage(page);
		await page.goto("./feeds/visual-preview");

		const card = feedsPage.getFeedCardByTitle("AI Trends");
		await expect(card).toBeVisible({ timeout: 15000 });

		// The whole point: the image arrives even though the feed list carried no
		// proxy URL, and it arrives without the card ever having an articleId.
		await expect(card.locator('[data-testid="card-image"]')).toBeVisible({
			timeout: 15000,
		});
		await expect(card.locator('[data-testid="image-fallback"]')).toHaveCount(0);

		expect(resolveRequests.length).toBeGreaterThan(0);
		expect(resolveRequests.flat()).toContain("feed-1");
	});

	test("an unresolvable feed settles on the fallback and is requested exactly once", async ({
		page,
	}) => {
		await stubFeedLists(page);

		let resolveCalls = 0;
		await page.route(CONNECT_RPC_PATHS.resolveOgImages, async (route) => {
			resolveCalls += 1;
			// The origin refused (robots.txt disallow, or a 403/404). The server
			// records the failure; the response carries no URL for that feed.
			await fulfillJson(route, { images: [] });
		});

		await page.route("**/v1/images/proxy/**", (route) =>
			route.fulfill({ status: 200, contentType: "image/png", body: PNG_1x1 }),
		);

		const feedsPage = new DesktopFeedsPage(page);
		await page.goto("./feeds/visual-preview");

		const card = feedsPage.getFeedCardByTitle("AI Trends");
		await expect(card).toBeVisible({ timeout: 15000 });

		await expect(card.locator('[data-testid="image-fallback"]')).toBeVisible({
			timeout: 15000,
		});

		const callsAfterFirstPass = resolveCalls;
		expect(callsAfterFirstPass).toBeGreaterThan(0);

		// Scrolling away and back must not re-ask. A dead origin re-scraped on
		// every scroll is the crawl behaviour this change exists to remove.
		await page.mouse.wheel(0, 2000);
		await page.waitForTimeout(500);
		await page.mouse.wheel(0, -2000);
		await expect(card).toBeVisible();
		await page.waitForTimeout(1000);

		expect(resolveCalls).toBe(callsAfterFirstPass);
	});

	test("cards the reader never reaches are never resolved", async ({ page }) => {
		// Zero-padded so "Article 01" cannot also match "Article 10".
		const manyFeeds = {
			data: Array.from({ length: 24 }, (_, i) => {
				const n = String(i + 1).padStart(2, "0");
				return feedWithoutOg(`feed-${n}`, `Article ${n}`, `article-${n}`);
			}),
			nextCursor: "",
			hasMore: false,
		};

		await page.route(CONNECT_RPC_PATHS.getAllFeeds, (r) =>
			fulfillJson(r, manyFeeds),
		);
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (r) =>
			fulfillJson(r, manyFeeds),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (r) =>
			fulfillJson(r, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		const requestedIds = new Set<string>();
		await page.route(CONNECT_RPC_PATHS.resolveOgImages, async (route) => {
			const body = route.request().postDataJSON() as { feedIds?: string[] };
			for (const id of body?.feedIds ?? []) requestedIds.add(id);
			await fulfillJson(route, {
				images: (body?.feedIds ?? []).map((feedId) => ({
					feedId,
					ogImageProxyUrl: proxyUrl(`https://img.example.com/${feedId}.png`),
				})),
			});
		});
		await page.route("**/v1/images/proxy/**", (route) =>
			route.fulfill({ status: 200, contentType: "image/png", body: PNG_1x1 }),
		);

		const feedsPage = new DesktopFeedsPage(page);
		await page.goto("./feeds/visual-preview");

		await expect(feedsPage.getFeedCardByTitle("Article 01")).toBeVisible({
			timeout: 15000,
		});
		// The first card must actually have been asked for, otherwise this test
		// would pass simply because nothing resolves at all.
		await expect
			.poll(() => requestedIds.has("feed-01"), { timeout: 15000 })
			.toBe(true);

		// The tail of a 24-card grid is well past the viewport and its 200px
		// margin; asking for it would be a crawl by another name.
		expect(requestedIds.has("feed-24")).toBe(false);
	});
});

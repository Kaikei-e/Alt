import {
	buildResolveOgImagesResponse,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { expect, test } from "../../fixtures/pomFixtures";
import { DesktopFeedsPage } from "../../pages/desktop/DesktopFeedsPage";
import { fulfillJson } from "../../utils/mockHelpers";

/**
 * A resolution that finishes after the reader's request has already died.
 *
 * `ResolveOgImages` fetches the publisher's page inline, behind a per-host
 * slot. A batch that cannot finish inside one RPC leaves the reader holding a
 * dead request while the feeds the server *did* reach are written to the store.
 * The card used to read that as the origin's answer and show "no preview" for
 * the rest of the session, with the picture sitting one cheap re-ask away — the
 * grid only filled in on a full page reload.
 *
 * Contract this spec pins:
 *  - a failure to reach the server is not the origin's answer: the card re-asks;
 *  - the image appears with no reload and no navigation;
 *  - the fallback is never shown on the way — a reader must not be told "this
 *    article has no picture" and then handed one;
 *  - the ladder is bounded: an unreachable server lands the card on the
 *    fallback rather than shimmering forever.
 */

// 1x1 transparent PNG.
const PNG_1x1 = Buffer.from(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==",
	"base64",
);

function feedWithoutOg(id: string, title: string, slug: string) {
	return {
		id,
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
	data: [feedWithoutOg("feed-1", "AI Trends", "ai-trends")],
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

/**
 * Record every image state the grid passes through.
 *
 * A retrying `not.toBeVisible()` assertion cannot prove the fallback was never
 * shown — it passes the moment the element is gone. Watching the DOM for the
 * whole run can.
 */
async function recordImageStates(page: import("@playwright/test").Page) {
	await page.addInitScript(() => {
		const seen = new Set<string>();
		(window as unknown as { __imageStates: Set<string> }).__imageStates = seen;
		const scan = () => {
			for (const testid of ["image-fallback", "image-loading", "card-image"]) {
				if (document.querySelector(`[data-testid="${testid}"]`)) {
					seen.add(testid);
				}
			}
		};
		// `document`, not `document.documentElement`: an init script runs at
		// document start, where the root element does not exist yet.
		new MutationObserver(scan).observe(document, {
			subtree: true,
			childList: true,
			attributes: true,
		});
		scan();
	});
}

function seenStates(page: import("@playwright/test").Page): Promise<string[]> {
	return page.evaluate(() => [
		...(window as unknown as { __imageStates: Set<string> }).__imageStates,
	]);
}

test.describe("Visual Preview — a resolution that lands late", () => {
	test("re-asks after a failure to reach the server and fills the card in", async ({
		page,
	}) => {
		await stubFeedLists(page);
		await recordImageStates(page);

		let calls = 0;
		await page.route(CONNECT_RPC_PATHS.resolveOgImages, async (route) => {
			calls += 1;
			if (calls === 1) {
				// The batch could not finish inside the RPC. Nothing was learned
				// about the feed — the server keeps its own answer.
				await route.fulfill({
					status: 503,
					contentType: "application/json",
					body: JSON.stringify({ code: "unavailable", message: "slot wait" }),
				});
				return;
			}
			const body = route.request().postDataJSON() as { feedIds?: string[] };
			const ids = body?.feedIds ?? [];
			await fulfillJson(route, buildResolveOgImagesResponse({ resolved: ids }));
		});

		await page.route("**/v1/images/proxy/**", (route) =>
			route.fulfill({ status: 200, contentType: "image/png", body: PNG_1x1 }),
		);

		const feedsPage = new DesktopFeedsPage(page);
		await page.goto("./feeds/visual-preview");

		const card = feedsPage.getFeedCardByTitle("AI Trends");
		await expect(card).toBeVisible({ timeout: 15000 });

		// The picture arrives on the second ask, in the page the reader is
		// already looking at.
		await expect(card.locator('[data-testid="card-image"]')).toBeVisible({
			timeout: 20000,
		});
		expect(calls).toBeGreaterThanOrEqual(2);

		// And it was never preceded by "no preview". A card that says the
		// article has no picture and then shows one has told the reader
		// something untrue.
		expect(await seenStates(page)).not.toContain("image-fallback");
	});

	test("gives up on an unreachable server rather than shimmering forever", async ({
		page,
	}) => {
		await stubFeedLists(page);

		let calls = 0;
		await page.route(CONNECT_RPC_PATHS.resolveOgImages, async (route) => {
			calls += 1;
			await route.fulfill({
				status: 503,
				contentType: "application/json",
				body: JSON.stringify({ code: "unavailable", message: "down" }),
			});
		});
		await page.route("**/v1/images/proxy/**", (route) =>
			route.fulfill({ status: 200, contentType: "image/png", body: PNG_1x1 }),
		);

		const feedsPage = new DesktopFeedsPage(page);
		await page.goto("./feeds/visual-preview");

		const card = feedsPage.getFeedCardByTitle("AI Trends");
		await expect(card).toBeVisible({ timeout: 15000 });

		await expect(card.locator('[data-testid="image-fallback"]')).toBeVisible({
			timeout: 25000,
		});

		// Bounded: the ladder is three asks, not an open-ended poll of a server
		// that is not answering.
		const settled = calls;
		await page.waitForTimeout(3000);
		expect(calls).toBe(settled);
		expect(calls).toBeLessThanOrEqual(3);
	});
});

import { expect, test } from "@playwright/test";
import { gotoMobileRoute } from "../../helpers/navigation";
import { fulfillJson } from "../../utils/mockHelpers";
import {
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";

const SUBSCRIPTIONS_EMPTY = { subscriptions: [] };

/**
 * Phase 5 regression: visual-preview SSR no longer streams the articleData
 * promise. The deferred-promise transport produced chunked-transfer tails
 * that iOS Safari occasionally rendered as "Cannot Open the Page" under
 * slow upstream tails. The load function now awaits the article fetch and
 * returns one fully-formed HTML payload.
 *
 * This spec only asserts the SSR HTML shape — it does not require an iOS
 * device. The chunk-recovery branch of D_top is covered by hooks.client
 * unit tests.
 */
test.describe("visual-preview SSR returns a non-streamed payload", () => {
	test("SSR HTML body terminates cleanly without a deferred promise tail", async ({
		page,
	}) => {
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) =>
			fulfillJson(route, CONNECT_FEEDS_RESPONSE),
		);
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);
		await page.route(
			"**/api/v2/alt.feeds.v2.FeedService/ListSubscriptions",
			(route) => fulfillJson(route, SUBSCRIPTIONS_EMPTY),
		);
		await page.route(CONNECT_RPC_PATHS.fetchArticleContent, (route) =>
			fulfillJson(route, { content: "", articleId: "", url: "" }),
		);

		const response = await gotoMobileRoute(page, "/feeds/swipe/visual-preview");
		expect(response?.status()).toBeLessThan(400);

		const body = await response!.text();
		// The deferred-promise transport injects a script tag with the literal
		// substring `__sveltekit_` and a Promise placeholder. The await-based
		// load function inlines articleData directly into the JSON payload,
		// so we should not see a streamed-promise marker for articleData.
		expect(body).not.toMatch(/__sveltekit_\d+\.resolve\(/);

		// And the body must terminate with the closing </html> tag so iOS
		// Safari sees a complete document.
		expect(body.trim().endsWith("</html>")).toBe(true);
	});
});

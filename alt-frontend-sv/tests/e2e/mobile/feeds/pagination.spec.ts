import { expect, test } from "../../fixtures/pomFixtures";
import {
	CONNECT_FEEDS_RESPONSE,
	CONNECT_READ_FEEDS_EMPTY_RESPONSE,
	CONNECT_RPC_PATHS,
} from "../../fixtures/mockData";
import { fulfillJson } from "../../utils/mockHelpers";

/**
 * Regression: the mobile feed list must stop paginating when the backend says
 * there is no next page.
 *
 * Connect-RPC serialises an absent cursor as the empty string — proto3 has no
 * nullable `string` — so a last page arrives as `{ nextCursor: "", hasMore:
 * false }`. `FeedsClient` decided whether to keep paging with
 * `next_cursor !== null`, which is *true* for `""`. The infinite-scroll sentinel
 * therefore re-requested the first page forever, appending feeds it already had
 * until Svelte started reporting `each_key_duplicate` on the `(feed.link)` keyed
 * block.
 *
 * The two assertions below are the contract: one page requested, and every
 * rendered feed distinct.
 */

/** Terminal page exactly as a Connect backend serialises it. */
const LAST_PAGE = {
	...CONNECT_FEEDS_RESPONSE,
	nextCursor: "",
	hasMore: false,
};

test.describe("mobile feeds pagination", () => {
	test("stops requesting pages once the cursor is exhausted", async ({
		page,
		mobileFeedsPage,
	}) => {
		let feedRequests = 0;
		await page.route(CONNECT_RPC_PATHS.getUnreadFeeds, (route) => {
			feedRequests += 1;
			return fulfillJson(route, LAST_PAGE);
		});
		await page.route(CONNECT_RPC_PATHS.getReadFeeds, (route) =>
			fulfillJson(route, CONNECT_READ_FEEDS_EMPTY_RESPONSE),
		);

		await mobileFeedsPage.goto();
		await expect(page.getByTestId("feed-card-container").first()).toBeVisible({
			timeout: 10_000,
		});

		// Give the infinite-scroll sentinel room to fire if it is going to. There
		// is no "pagination has settled" event to await, so this polls the very
		// thing under test: the request count must reach a fixed point.
		await expect
			.poll(() => feedRequests, {
				timeout: 5_000,
				message: "feed page requests should settle at one",
			})
			.toBe(1);

		// Every rendered card must be a distinct feed. Duplicate keys are what
		// Svelte complains about, but a user sees the same article twice.
		const links = await page
			.getByTestId("feed-card-container")
			.getByRole("link")
			.evaluateAll((nodes) => nodes.map((n) => n.getAttribute("href")));
		expect(new Set(links).size).toBe(links.length);
	});
});

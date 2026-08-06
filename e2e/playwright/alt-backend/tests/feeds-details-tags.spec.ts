import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../src/http.js";
import { ZERO_UUID } from "../src/env.js";
import { feedTagsByIDSchema, feedTagsByURLSchema, secureErrorSchema } from "../src/schemas.js";

/**
 * Feed details and tag lookups — the port of `24-feeds-details-tags.hurl`.
 *
 * Request shapes, pinned against the Go structs:
 *   - `POST /v1/feeds/fetch/details` binds `FeedUrlPayload{FeedURL}` — a
 *     single `feed_url` string, NOT a `feed_urls` list. (The plural shape
 *     belongs to the unrelated `/fetch/summary` endpoints.)
 *   - `POST /v1/feeds/tags` binds `FeedTagsPayload{FeedURL}`, same singular
 *     convention.
 *   - `GET /v1/feeds/:id/tags` reads by DB UUID.
 *
 * An earlier generation of this scenario sent `feed_urls: [...]` to both POSTs,
 * which always bound an empty FeedURL, so both requests only ever exercised
 * the 400 validation branch — masked by a `status >= 200; status < 600`
 * assertion that accepted literally any outcome.
 */

test.describe("POST /v1/feeds/fetch/details", () => {
	test("a URL with no summary is a typed 404, not a silent 200", async ({
		rest,
		csrf,
		seededFeeds,
	}) => {
		// The summary query keys on *article* URLs, and feed URLs are never rows
		// in `articles.url`, so this is the miss branch.
		//
		// History worth keeping: the pre-split driver surfaced the miss as an
		// unwrapped pgx.ErrNoRows that fell through to a generic 500, and this
		// scenario pinned that wart to catch an accidental silent 200. The Wave
		// 3 data-hub migration (ADR-000954) then made the gateway report a miss
		// as (nil, nil) — exactly the silent 200 this pin existed to catch — so
		// the fix was implemented for real: FeedsSummaryUsecase maps a nil
		// summary to the typed FEED_NOT_FOUND, which HTTPStatusCode maps to 404.
		const feed = seededFeeds[0];
		expect(feed).toBeDefined();

		const body = await expectJsonStatus(
			await rest.post("/v1/feeds/fetch/details", {
				headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
				data: { feed_url: feed?.url ?? "" },
			}),
			404,
			secureErrorSchema,
		);
		expect(body.error.code).toBe("FEED_NOT_FOUND");
	});

	test("a plural feed_urls body is rejected", async ({ rest, csrf }) => {
		// The regression fence for the bug described in the file header: if
		// someone "fixes" the handler to accept the plural shape, this fails and
		// the conversation happens before the singular contract silently drifts.
		const response = await rest.post("/v1/feeds/fetch/details", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { feed_urls: ["http://stub.invalid/alt-backend/e2e/whatever.xml"] },
		});
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});
});

test.describe("POST /v1/feeds/tags", () => {
	test("a registered feed with no tags returns an empty list", async ({
		rest,
		csrf,
		seededFeeds,
	}) => {
		// FetchFeedTagsGateway.FetchFeedTags is a list query (unlike the
		// single-row QueryRow above), so zero tags on a real feed is a normal
		// empty result, not an error.
		const feed = seededFeeds[0];
		expect(feed).toBeDefined();
		const url = feed?.url ?? "";

		const body = await expectJsonStatus(
			await rest.post("/v1/feeds/tags", {
				headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
				data: { feed_url: url },
			}),
			200,
			feedTagsByURLSchema,
		);
		expect(body.feed_url).toBe(url);
		expect(body.tags.length).toBe(0);
	});
});

test.describe("GET /v1/feeds/:id/tags", () => {
	test("an unknown feed id returns an empty list, not a 404", async ({ rest }) => {
		// FetchFeedTagsByIDUsecase does no existence check before the list
		// query, so this is 200 with an empty array (mirrors
		// handleFetchArticleTags's by-ID behaviour). Pinning it here means a
		// future "add a 404" change has to be a deliberate one.
		const body = await expectJsonStatus(
			await rest.get(`/v1/feeds/${ZERO_UUID}/tags`),
			200,
			feedTagsByIDSchema,
		);
		expect(body.feed_id).toBe(ZERO_UUID);
		expect(body.tags.length).toBe(0);
	});

	test("KNOWN BUG: a malformed feed id reaches the query layer and 500s", async ({ rest }) => {
		// New coverage, and it found something. `RestHandleFetchFeedTagsByID`
		// checks only that `:id` is non-empty (details.go:262-266) — there is no
		// `uuid.Parse`, unlike every sibling handler that takes an id
		// (`RestHandleDeleteRSSFeedLink`, `handleGetScrapingDomain`,
		// `handleRefreshRobotsTxt`). So the raw path segment is handed to the
		// query and Postgres rejects it as an invalid uuid input syntax, which
		// surfaces as a generic 500.
		//
		// A caller-controlled string reaching the query layer is worth naming
		// even when the driver parameterises it: the 400-vs-500 split is what
		// tells a client "you sent nonsense" from "we broke". Pinning the
		// current answer means the fix — adding `uuid.Parse` and returning
		// HandleValidationError — fails this test and gets it updated.
		const response = await rest.get("/v1/feeds/not-a-uuid/tags");
		await expectStatus(response, 500);
	});
});

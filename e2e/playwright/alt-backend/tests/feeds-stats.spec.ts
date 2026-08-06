import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { detailedFeedStatsSchema, feedStatsSchema } from "../src/schemas.js";

/**
 * Aggregate feed stats — the port of `25-feeds-stats.hurl`.
 *
 * All three endpoints read alt_db through the data hub and call no external
 * service, so they are the fastest and most deterministic surface in the
 * suite. The Hurl file asserted `jsonpath "$" exists` on each — true of `{}`,
 * of `null`, and of an error envelope alike. Here the counter envelopes are
 * asserted field by field, which is what makes a stats endpoint that silently
 * started returning an empty object fail.
 */

test.describe("GET /v1/feeds/stats", () => {
	test("returns the feed and summarised counters", async ({ rest, seededFeeds }) => {
		expect(seededFeeds.length).toBe(3);

		const stats = await expectJsonStatus(await rest.get("/v1/feeds/stats"), 200, feedStatsSchema);

		// `SELECT COUNT(*) FROM feeds` — feed *items*, ingested when this
		// worker's three registrations parsed the stub's RSS documents. The
		// assertion is a lower bound of 1 rather than an exact number because
		// the query carries no tenant predicate and sibling workers push it up;
		// what it catches is the failure that matters, a stats endpoint that
		// answers 200 with everything zeroed.
		expect(stats.feed_amount.amount).toBeGreaterThanOrEqual(1);
	});
});

test.describe("GET /v1/feeds/stats/detailed", () => {
	test("returns the feed, article and unsummarised counters", async ({ rest, seededFeeds }) => {
		expect(seededFeeds.length).toBe(3);

		const stats = await expectJsonStatus(
			await rest.get("/v1/feeds/stats/detailed"),
			200,
			detailedFeedStatsSchema,
		);
		expect(stats.feed_amount.amount).toBeGreaterThanOrEqual(1);
	});
});

test.describe("GET /v1/feeds/stats/trends", () => {
	test("answers with a JSON body", async ({ rest }) => {
		// The trend envelope is shaped by the window it computes over, which the
		// staging fixture set does not populate; what is contractual here is
		// that the endpoint answers 200 with JSON rather than faulting on an
		// empty window.
		const response = await rest.get("/v1/feeds/stats/trends");
		await expectStatus(response, 200);
		expect(await response.json()).not.toBeNull();
	});
});

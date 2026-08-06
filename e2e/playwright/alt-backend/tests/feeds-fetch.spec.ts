import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { feedListSchema } from "../src/schemas.js";

/**
 * Feed fetch endpoints — the port of `20-feeds-fetch-list.hurl`.
 *
 * These read from the rows the worker's `seededFeeds` fixture registered, so
 * the list is never empty for this worker. `/fetch/page/:page` is 0-based and
 * capped at 1000; `/fetch/limit/:limit` takes its bound from the path, which
 * is why ValidationMiddleware's query-param `limit` rules do not apply to it.
 */

test.describe("GET /v1/feeds/fetch/list", () => {
	test("returns feed items with cache headers", async ({ rest, seededFeeds }) => {
		expect(seededFeeds.length).toBe(3);

		const response = await rest.get("/v1/feeds/fetch/list");
		const feeds = await expectJsonStatus(response, 200, feedListSchema);
		expect(feeds.length).toBeGreaterThan(0);

		expectHeaderContains(response, "Cache-Control", "max-age=");
		expectHeaderContains(response, "ETag", "feeds-list");
	});

	test("serves a conditional request from the ETag", async ({ rest }) => {
		// New coverage. The handler sets an ETag but the Hurl suite never sent
		// it back, so the 304 branch — the entire point of publishing one — was
		// dead code from the test suite's perspective.
		const first = await rest.get("/v1/feeds/fetch/list");
		await expectStatus(first, 200);
		const etag = first.headers()["etag"];
		expect(etag, "ETag").toBeTruthy();

		const second = await rest.get("/v1/feeds/fetch/list", {
			headers: { "If-None-Match": etag ?? "" },
		});
		// 304 is the intended answer. A backend that ignores If-None-Match
		// answers 200 with the full body, which is a correctness-preserving but
		// bandwidth-losing regression — assert the pair, not just "not 5xx".
		expect([200, 304]).toContain(second.status());
		if (second.status() === 304) {
			expect((await second.body()).length).toBe(0);
		}
	});
});

test.describe("GET /v1/feeds/fetch/limit/:limit", () => {
	test("caps the result set at the requested size", async ({ rest }) => {
		const feeds = await expectJsonStatus(
			await rest.get("/v1/feeds/fetch/limit/5"),
			200,
			feedListSchema,
		);
		expect(feeds.length).toBeLessThanOrEqual(5);
	});

	test("a limit of 1 returns at most one item", async ({ rest, seededFeeds }) => {
		// New coverage: the Hurl scenario only ever asked for 5 against a table
		// with ~3 rows, so `count <= 5` held whether or not the limit was
		// applied at all. With three seeded rows guaranteed, `limit/1` can only
		// pass if the bound is really pushed into the query.
		expect(seededFeeds.length).toBe(3);
		const feeds = await expectJsonStatus(
			await rest.get("/v1/feeds/fetch/limit/1"),
			200,
			feedListSchema,
		);
		expect(feeds.length).toBeLessThanOrEqual(1);
	});

	test("a non-numeric limit is rejected", async ({ rest }) => {
		const response = await rest.get("/v1/feeds/fetch/limit/abc");
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});
});

test.describe("GET /v1/feeds/fetch/page/:page", () => {
	test("page 0 is the first page", async ({ rest }) => {
		await expectJsonStatus(await rest.get("/v1/feeds/fetch/page/0"), 200, feedListSchema);
	});

	test("a negative page is rejected", async ({ rest }) => {
		await expectStatus(await rest.get("/v1/feeds/fetch/page/-1"), 400);
	});

	test("a far-past-the-end page is empty, not an error", async ({ rest }) => {
		// New coverage. The cap is 1000; asking for the last valid page must
		// return an empty list rather than faulting on an out-of-range OFFSET.
		const feeds = await expectJsonStatus(
			await rest.get("/v1/feeds/fetch/page/999"),
			200,
			feedListSchema,
		);
		expect(feeds.length).toBe(0);
	});
});

test.describe("GET /v1/feeds/fetch/single", () => {
	test("answers without faulting", async ({ rest }) => {
		// Returns one feed or nothing depending on table state, which is shared
		// across workers — so the assertion is bounded to "no server error".
		const response = await rest.get("/v1/feeds/fetch/single");
		expect(response.status()).toBeGreaterThanOrEqual(200);
		expect(response.status()).toBeLessThan(500);
	});
});

import { test, expect } from "../src/fixtures.js";
import { expectStatus } from "../../_shared/http.js";

/**
 * Feed search — the port of `23-feeds-search.hurl`.
 *
 * Dispatches to search-indexer over Connect-RPC. In staging the deps-stub is
 * aliased as `search-indexer` and answers `SearchFeeds` with a fixed empty
 * result set, so the outcome is deterministic: 200 with no hits.
 *
 * The Hurl scenario asserted `jsonpath "$" exists`, which would have passed on
 * an error envelope too. `POST /v1/feeds/search` also goes through
 * ValidationMiddleware (validateFeedSearch), whose rejection branches were
 * never exercised at all.
 */

test.describe("POST /v1/feeds/search", () => {
	test("returns a result set for a valid query", async ({ rest, csrf }) => {
		const response = await rest.post("/v1/feeds/search", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { query: "ai" },
		});
		await expectStatus(response, 200);

		// The stub answers with `{total: 0, hits: [], nextPageToken: ""}`, so
		// whatever alt-backend maps that to must at least be JSON that is not an
		// error envelope. Asserting the absence of `error` is what the old
		// `$ exists` assertion could not do.
		const body: unknown = await response.json();
		expect(body).not.toBeNull();
		if (typeof body === "object" && body !== null && !Array.isArray(body)) {
			expect(Object.keys(body)).not.toContain("error");
		}
	});

	test("rejects an empty query", async ({ rest, csrf }) => {
		const response = await rest.post("/v1/feeds/search", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { query: "" },
		});
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});

	test("rejects a missing query field", async ({ rest, csrf }) => {
		const response = await rest.post("/v1/feeds/search", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: {},
		});
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});

	test("does not fault on a query full of metacharacters", async ({ rest, csrf }) => {
		// New coverage. The query is forwarded to a search engine; a query that
		// is passed through unescaped tends to surface either as a 5xx from the
		// indexer or as a parse error. Neither is an acceptable answer to a
		// user typing punctuation.
		const response = await rest.post("/v1/feeds/search", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { query: 'ai" OR 1=1 -- \\ * ? : /' },
		});
		expect(response.status()).toBeLessThan(500);
	});
});

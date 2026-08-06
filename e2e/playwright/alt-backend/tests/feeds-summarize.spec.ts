import { test, expect, stubURL } from "../src/fixtures.js";
import { expectStatus, expectStatusIn } from "../../_shared/http.js";
import { ZERO_UUID } from "../src/env.js";

/**
 * Summarisation queue and stream — the port of
 * `27-feeds-summarize-queue.hurl`, plus the `/summarize/stream` endpoint that
 * file explicitly listed as "excluded from the initial suite".
 *
 * Why the two enqueue paths cannot succeed in staging: both call
 * `EnsureArticle` first, which fetches the article body, and the SSRF guard
 * rejects any `stub.invalid` URL there — it resolves to a Docker-internal
 * address, which the guard reports as DNS_REBINDING_BLOCKED. The handler maps
 * that to a 500 via `echo.NewHTTPError`, so the body is not the AppError
 * envelope other scenarios assert and is not reliably JSON at all. Only the
 * status is pinned.
 *
 * The bands below exclude 2xx (a success would mean the guard or the stub host
 * changed and these scenarios need revisiting) and 4xx (which would mean the
 * request shape regressed).
 */

const ENQUEUE_ENDPOINTS = [
	{ name: "synchronous", path: "/v1/feeds/summarize" },
	{ name: "queued", path: "/v1/feeds/summarize/queue" },
] as const;

test.describe("enqueue", () => {
	for (const endpoint of ENQUEUE_ENDPOINTS) {
		test(`${endpoint.name}: dies in the SSRF guard inside EnsureArticle`, async ({
			rest,
			csrf,
			seededFeeds,
		}) => {
			const feed = seededFeeds[0];
			const response = await rest.post(endpoint.path, {
				headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
				data: {
					// Both handlers bind `feed_url` only — the anonymous struct in
					// request_handlers.go has no `content` field, so the literal below
					// keeps it for readability but it is ignored server-side.
					feed_url: feed?.url ?? stubURL("feed-summarize-fallback.xml"),
					content: "Stub article body for summarization.",
				},
			});
			await expectStatusIn(response, [500, 503, 504]);
		});
	}
});

test.describe("job status", () => {
	test("polling an unknown job id reaches the pre-processor", async ({ rest }) => {
		// Status polling fetches nothing, so unlike the enqueue paths it really
		// does reach the pre-processor client. A real pre-processor answers 404
		// for an unknown job and the handler surfaces that; the deps-stub's
		// catch-all answers 200 instead, which the client accepts. Both are
		// legitimate depending on what sits behind PRE_PROCESSOR_URL, so the
		// band covers both rather than encoding which one the stub happens to be.
		const response = await rest.get(`/v1/feeds/summarize/status/${ZERO_UUID}`);
		await expectStatusIn(response, [200, 404]);
	});

	test("a malformed job id does not reach the upstream as-is", async ({ rest }) => {
		const response = await rest.get("/v1/feeds/summarize/status/not-a-uuid");
		expect(response.status()).toBeLessThan(500);
	});
});

/**
 * `/v1/feeds/summarize/stream` — entirely new coverage.
 *
 * The Hurl suite excluded it with "NDJSON chunked-transfer framing is out of
 * scope", which left three pieces of hand-written middleware configuration
 * untested, each of which exists *only* for this route:
 *
 *   1. `BodyLimitWithConfig`'s Skipper (`/stream`) — H-005.
 *   2. `TimeoutWithConfig`'s Skipper (`/stream`).
 *   3. The DoS whitelist entry `/v1/feeds/summarize/stream` — M-005 made that
 *      an exact match rather than a substring, so a route rename silently
 *      removes the exemption.
 *
 * None of those needs the stream to actually produce tokens to be observable,
 * which is what makes them testable here without a real pre-processor.
 */
test.describe("streaming summarisation", () => {
	test("requires either feed_url or article_id", async ({ rest, csrf }) => {
		// The one fully deterministic branch: the handler rejects before it
		// resolves anything, so no upstream is involved.
		const response = await rest.post("/v1/feeds/summarize/stream", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { content: "body without an identifier" },
		});
		await expectStatus(response, 400);
		expect(await response.text()).toContain("feed_url");
	});

	test("rejects a malformed JSON body", async ({ rest, csrf }) => {
		const response = await rest.post("/v1/feeds/summarize/stream", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: "{ not json",
		});
		await expectStatus(response, 400);
	});

	test("is still behind authentication", async ({ restAnon, csrf }) => {
		// Being on the DoS whitelist exempts the route from rate limiting, not
		// from auth. Conflating the two is a plausible refactor mistake and an
		// expensive one.
		const response = await restAnon.post("/v1/feeds/summarize/stream", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { feed_url: stubURL("feed-stream-anon.xml"), content: "x" },
		});
		await expectStatus(response, 401);
	});

	test("is exempt from the 2MB body limit", async ({ rest, csrf }) => {
		// H-005's Skipper. The same payload on a non-streaming route is a 413
		// (asserted in tests/security-headers.spec.ts); here it must get past
		// the limiter and be answered by the handler instead. Any status other
		// than 413 proves the skipper fired — what it then answers depends on
		// the upstream, which is not what this test is about.
		const oversize = "x".repeat(3 * 1024 * 1024);
		const response = await rest.post("/v1/feeds/summarize/stream", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { feed_url: stubURL("feed-stream-oversize.xml"), content: oversize },
		});
		expect(
			response.status(),
			"413 means the /stream skipper stopped matching — check BodyLimitConfig.Skipper",
		).not.toBe(413);
	});

	test("resolving a stub article fails in the SSRF guard, not in a panic", async ({
		rest,
		csrf,
		seededFeeds,
	}) => {
		// ResolveStreamArticle fetches the body, so this hits the same guard as
		// the enqueue paths. What is asserted is that the failure is an ordinary
		// HTTP error and not a half-written stream: a handler that has already
		// called setStreamingHeaders before failing cannot report a status at
		// all, which is the failure mode worth catching here.
		const feed = seededFeeds[0];
		const response = await rest.post("/v1/feeds/summarize/stream", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { feed_url: feed?.url ?? stubURL("feed-stream-fallback.xml") },
		});
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(600);
	});
});

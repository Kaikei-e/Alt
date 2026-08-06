import { test, expect } from "../src/fixtures.js";
import { expectJson, expectStatus } from "../src/http.js";
import { connectErrorSchema } from "../src/schemas.js";

/**
 * User-facing Connect-RPC mux — new coverage.
 *
 * `SetupConnectHandlers` mounts nine services behind one shared
 * `AuthInterceptor`. The Hurl suite probed exactly one of them
 * (`KnowledgeTrailService/GetTrail`) plus one procedure on a second, which
 * meant a `mux.Handle` line deleted in a refactor — or a handler whose DI
 * dependency came back nil and skipped registration — would have gone
 * unnoticed until the SPA broke in production.
 *
 * This is CLAUDE.md rule 8's failure mode ("DI forgot to wire" is
 * indistinguishable from "intentionally disabled") pushed out to the E2E
 * boundary, where the only thing that can tell the two apart is whether the
 * path resolves at all.
 *
 * The discriminator is 401-vs-404:
 *   - **401** — the AuthInterceptor ran, so the procedure is mounted.
 *   - **404** — the path resolved to nothing; the service is not registered.
 * A test that merely asserted "not 2xx" would accept both and prove nothing.
 */

/** One procedure per service registered by SetupConnectHandlers. */
const MOUNTED_SERVICES = [
	{ service: "FeedService", procedure: "alt.feeds.v2.FeedService/GetAllFeeds" },
	{ service: "ArticleService", procedure: "alt.articles.v2.ArticleService/FetchArticlesCursor" },
	{ service: "RSSService", procedure: "alt.rss.v2.RSSService/ListRSSFeedLinks" },
	{ service: "AugurService", procedure: "alt.augur.v2.AugurService/ListConversations" },
	{ service: "MorningLetterReadService", procedure: "alt.morning_letter.v2.MorningLetterReadService/GetLatestLetter" },
	{ service: "RecapService", procedure: "alt.recap.v2.RecapService/GetThreeDayRecap" },
	{ service: "KnowledgeHomeService", procedure: "alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome" },
	{ service: "KnowledgeTrailService", procedure: "alt.knowledge_trail.v1.KnowledgeTrailService/GetTrail" },
] as const;

test.describe("Connect-RPC service registration", () => {
	for (const { service, procedure } of MOUNTED_SERVICES) {
		test(`${service} is mounted on :9101`, async ({ connectAnon }) => {
			const response = await connectAnon.post(`/${procedure}`, { data: {} });
			expect(
				response.status(),
				`${procedure} answered ${response.status()}. 404 means the handler was ` +
					`never registered (check SetupConnectHandlers and the DI container); ` +
					`401 is the expected "mounted, but you are anonymous".`,
			).toBe(401);
		});

		test(`${service} accepts an authenticated call`, async ({ connect }) => {
			// Whatever the procedure does with an empty request — succeed,
			// invalid_argument, not_found — it must not answer 404 or 401 to a
			// valid admin JWT. Those two are wiring failures, not business
			// outcomes.
			const response = await connect.post(`/${procedure}`, { data: {} });
			expect([404, 401]).not.toContain(response.status());
		});
	}
});

test.describe("Connect-RPC error envelope", () => {
	test("an unmounted procedure 404s from the Go mux, not the Connect layer", async ({
		connect,
	}) => {
		// Pinned after the first CI run: the body is Go's plain-text
		// `404 page not found`, not a Connect error envelope. connect-go's
		// generated `NewFeedServiceHandler` routes only the procedures it knows
		// and hands anything else to `http.NotFound`, so an unknown *procedure*
		// on a known *service* never reaches the Connect codec at all.
		//
		// This matters to the frontend: connect-es surfaces this as a transport
		// error rather than a `ConnectError` with a numeric code, so a
		// client-side handler that switches on `ConnectError.code` will not see
		// `unimplemented` here. Asserting the plain text keeps that fact
		// visible instead of implying an envelope that does not exist.
		const response = await connect.post("/alt.feeds.v2.FeedService/NoSuchProcedure", {
			data: {},
		});
		await expectStatus(response, 404);
		expect(await response.text()).toContain("404 page not found");
	});

	test("an unauthenticated call answers a Connect-shaped 401", async ({ connectAnon }) => {
		// The frontend's connect-es client switches on the numeric ConnectError
		// code, which it can only derive from a well-formed error envelope. A
		// bare Echo-style `{"error": "..."}` here would be a silent client-side
		// break, so the envelope shape is part of the contract.
		const response = await connectAnon.post("/alt.feeds.v2.FeedService/GetAllFeeds", {
			data: {},
		});
		await expectStatus(response, 401);
		const body = await expectJson(response, connectErrorSchema);
		expect(body.code).toBe("unauthenticated");
	});

	test("a malformed JSON body answers invalid_argument, not a 500", async ({ connect }) => {
		const response = await connect.post("/alt.feeds.v2.FeedService/GetAllFeeds", {
			headers: { "Content-Type": "application/json" },
			data: "{ this is not json",
		});
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(500);
	});
});

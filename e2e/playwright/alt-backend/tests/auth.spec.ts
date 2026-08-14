import { test } from "../src/fixtures.js";
import { expectStatus } from "../../_shared/http.js";
import { WRONG_SIGNATURE_JWT, ZERO_UUID } from "../src/env.js";

/**
 * JWT auth middleware, negative paths — the port of `02-auth-negative.hurl`,
 * widened from one endpoint to every protected route family.
 *
 * The Hurl file probed `/v1/feeds/fetch/list` three times and left the other
 * eight groups untested, so a route registered on the wrong Echo group — one
 * without `authMiddleware.RequireAuth()` — would have shipped green. The
 * table below has one entry per `Group(...)` in routes.go / rest_feeds/
 * routes.go, which is the granularity at which that mistake is actually made.
 *
 * Audience/issuer mismatches stay in jwt_middleware.go's unit tests; this file
 * lives at the middleware boundary.
 */

const PROTECTED_ROUTES = [
	{ group: "feeds", path: "/v1/feeds/fetch/list" },
	{ group: "feeds (cursor)", path: "/v1/feeds/fetch/cursor?limit=1" },
	{ group: "feeds (stats)", path: "/v1/feeds/stats" },
	{ group: "feeds (tags by id)", path: `/v1/feeds/${ZERO_UUID}/tags` },
	{ group: "rss-feed-link", path: "/v1/rss-feed-link/list" },
	{ group: "articles", path: "/v1/articles/fetch/cursor?limit=1" },
	{ group: "articles (tags by id)", path: `/v1/articles/${ZERO_UUID}/tags` },
	{ group: "morning-letter", path: "/v1/morning-letter/updates" },
	{ group: "images (proxy)", path: `/v1/images/proxy/invalidsig/${encodeURIComponent("http://stub.invalid/x.png")}` },
	{ group: "admin (dashboard)", path: "/v1/dashboard/metrics" },
	{ group: "admin (scraping-domains)", path: "/v1/admin/scraping-domains" },
	{ group: "augur (rag context)", path: "/v1/rag/context?q=ai" },
	// The augur pair's other half, `/sse/v1/rag/answer`, cannot join this table:
	// it is POST-only, so a GET is answered 405 by Echo's router before any
	// route middleware runs. It has its own test below.
] as const;

test.describe("JWT auth boundary", () => {
	for (const route of PROTECTED_ROUTES) {
		test(`${route.group}: no token → 401`, async ({ restAnon }) => {
			await expectStatus(await restAnon.get(route.path), 401);
		});
	}

	test("augur (sse answer): no token → 401", async ({ restAnon, csrf }) => {
		// The half of the augur pair that is mounted on the Echo root rather than
		// on the /v1 group — augur_handler.go keeps the `/sse/` prefix because the
		// BodyLimit, Timeout and Gzip skippers in routes.go all match on it — so
		// it takes RequireAuth as per-route middleware and is the one route here
		// whose guard is not inherited from a Group.
		//
		// The CSRF token is sent even though this is an auth test: CSRFMiddleware
		// is a global `e.Use` and therefore runs first, so without one the answer
		// is 403 and says nothing about JWT.
		const response = await restAnon.post("/sse/v1/rag/answer", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { messages: [{ role: "user", content: "hello" }], stream: false },
		});
		await expectStatus(response, 401);
	});

	test("malformed token → 401", async ({ restAnon }) => {
		const response = await restAnon.get("/v1/feeds/fetch/list", {
			headers: { "X-Alt-Backend-Token": "not-a-jwt" },
		});
		await expectStatus(response, 401);
	});

	test("well-formed token signed with the wrong secret → 401", async ({ restAnon }) => {
		// Payload identical to the real fixture, HMAC computed with a different
		// key. If this ever returns 200 the middleware stopped verifying the
		// signature and is only decoding claims.
		const response = await restAnon.get("/v1/feeds/fetch/list", {
			headers: { "X-Alt-Backend-Token": WRONG_SIGNATURE_JWT },
		});
		await expectStatus(response, 401);
	});

	test("token in the Authorization header alone → 401", async ({ restAnon }) => {
		// alt-backend reads X-Alt-Backend-Token, never Authorization. A future
		// "let's also accept Bearer" change is a widening of the auth surface
		// and must be a deliberate one, not something that slips in.
		const response = await restAnon.get("/v1/feeds/fetch/list", {
			headers: { Authorization: "Bearer not-a-jwt" },
		});
		await expectStatus(response, 401);
	});

	test("empty token header → 401", async ({ restAnon }) => {
		const response = await restAnon.get("/v1/feeds/fetch/list", {
			headers: { "X-Alt-Backend-Token": "" },
		});
		await expectStatus(response, 401);
	});
});

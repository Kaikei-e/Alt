import { test, expect } from "../src/fixtures.js";
import { expectStatus } from "../../_shared/http.js";
import { syntheticClientIP } from "../src/fixtures.js";

/**
 * The per-IP rate limiters — entirely new coverage.
 *
 * The Hurl README ruled this out on purpose: "Rate-limit coverage (429 /
 * Retry-After) is intentionally out of the Hurl suite — it lives in
 * auth-hub/middleware/rate_limit_test.go where wall-clock timing isn't at the
 * mercy of container cold-start." That was the right call *for Hurl*, because
 * a scenario file has no way to give itself a private client address and
 * therefore no way to avoid draining a bucket every other scenario depends on.
 *
 * Here every test gets its own synthetic `X-Forwarded-For` (src/fixtures.ts),
 * so a bucket can be drained to exhaustion without touching anything else, and
 * the assertions below are about *token-bucket capacity* rather than about
 * elapsed time — no test here waits for a refill. That removes the cold-start
 * fragility the Hurl suite was avoiding while keeping what the unit test
 * cannot see: that `main.go` attached the limiters to the routes it meant to,
 * with the burst sizes it computed, behind the IP extractor it configured.
 *
 * The unit test in `middleware/rate_limit_test.go` proves the limiter works.
 * This proves it is *wired*, which is the failure CLAUDE.md rule 8 is about.
 */

test.describe("rate limiting", () => {
	test("the /session limiter refuses past its burst @contract", async ({ hub }) => {
		// main.go:164-168: `SessionRateLimit` defaults to 30/60 = 0.5 req/s, and
		// `sessionBurst = int(0.5 * 10) = 5`, floored at 5. compose.staging.yaml
		// sets no override, so the bucket holds 5 tokens and refills one every
		// two seconds.
		//
		// These are anonymous requests: `sessionRL.Middleware()` is route
		// middleware and runs *before* the handler's cookie check, so a 401 is
		// proof the limiter let the request through and a 429 is proof it did
		// not. Twenty requests issued together cannot possibly be covered by five
		// tokens plus whatever trickles in over the round-trip, so at least one
		// 429 is a hard consequence rather than a timing hope.
		const responses = await Promise.all(
			Array.from({ length: 20 }, () => hub.get("/session")),
		);
		const statuses = responses.map((response) => response.status());

		expect(
			statuses.filter((status) => status === 429).length,
			`expected the /session bucket (burst 5) to refuse some of 20 concurrent ` +
				`requests; got ${JSON.stringify(statuses)}`,
		).toBeGreaterThan(0);

		// And nothing else: a 500 here would mean the limiter blew up rather than
		// refused, and a 200 would mean an anonymous caller got a session.
		expect(statuses.filter((status) => status !== 401 && status !== 429)).toEqual([]);
	});

	test("a refusal carries Retry-After @contract", async ({ hub }) => {
		// `rate_limit.go` sets `Retry-After: max(int(1/rate), 1)` before answering
		// 429. For the /session limiter that is `int(1/0.5)` = 2 seconds. Without
		// the header a client has no way to back off correctly, and a retry storm
		// against the identity provider is precisely what the limiter exists to
		// prevent — so the header missing is a worse bug than the limit being
		// slightly wrong.
		const responses = await Promise.all(
			Array.from({ length: 20 }, () => hub.get("/session")),
		);
		const refused = responses.find((response) => response.status() === 429);
		expect(refused, "at least one of 20 concurrent /session calls should be refused").toBeTruthy();

		const retryAfter = refused?.headers()["retry-after"];
		expect(retryAfter, "Retry-After on a 429").toBeDefined();
		expect(Number.parseInt(retryAfter ?? "", 10)).toBe(2);
	});

	test("the /internal limiter refuses past burst 3 @contract", async ({ hub }) => {
		// main.go:174 — `NewRateLimiter(10.0/60.0, 3)`, hard-coded and explicitly
		// not env-tunable, which the Hurl suite worked around with per-scenario
		// `retry-interval: 6000`. The limiter is group middleware installed ahead
		// of `wireInternalAuth`, so unauthenticated probes consume tokens too:
		// three land on the auth middleware and answer 401, the rest are refused.
		//
		// This is also the assertion that makes the ordering explicit. If the
		// limiter were ever moved behind the auth middleware, an unauthenticated
		// flood would stop being throttled at all — a denial-of-service amplifier
		// pointed at Kratos's admin API, reachable by anyone who can address the
		// port.
		const responses = await Promise.all(
			Array.from({ length: 10 }, () => hub.get("/internal/system-user")),
		);
		const statuses = responses.map((response) => response.status());

		expect(
			statuses.filter((status) => status === 429).length,
			`expected the /internal bucket (burst 3) to refuse most of 10 concurrent ` +
				`requests; got ${JSON.stringify(statuses)}`,
		).toBeGreaterThan(0);
		expect(statuses.filter((status) => status !== 401 && status !== 429)).toEqual([]);

		const refused = responses.find((response) => response.status() === 429);
		// `int(1 / (10/60))` = `int(6)` = 6.
		expect(Number.parseInt(refused?.headers()["retry-after"] ?? "", 10)).toBe(6);
	});

	test("the limiters are keyed per client address @contract", async ({ hub, hubFrom }) => {
		// The load-bearing test in this file, and the one that explains every
		// other failure in the suite if it breaks.
		//
		// Everything here assumes `e.IPExtractor = echo.ExtractIPFromXFFHeader()`
		// (main.go:114) honours the `X-Forwarded-For` this suite sets — the test
		// container's own address is RFC 1918 and therefore trusted, so the
		// extractor should walk past it to the 100.64/10 address in the header.
		// If that assumption is wrong, all four workers collapse onto one bucket
		// and the /session limiter (burst 5) starts refusing unrelated tests at
		// random. Asserting it directly turns that from a fleet of confusing
		// intermittent 429s into one named failure.
		//
		// Drain this test's own bucket first, then prove a *different* address is
		// unaffected. The second address is untouched, so it must answer 401 —
		// the limiter passed it through to the handler's cookie check.
		await Promise.all(Array.from({ length: 20 }, () => hub.get("/session")));

		// Worker index 63 puts the neighbour in 100.127.0.0/16, which no worker
		// can ever mint for itself — `syntheticClientIP` derives the second octet
		// from `workerIndex % 64` and this suite runs four workers, so the live
		// range is 100.64–100.67.
		const neighbour = await hubFrom(syntheticClientIP(63));
		const response = await neighbour.get("/session");
		await expectStatus(response, 401);
	});

	test("/health stays available while another route is throttled @contract", async ({ hub }) => {
		// The liveness guarantee. `/health` carries no limiter (main.go:180), so
		// draining the /session bucket for this client must not make the container
		// look dead to its own healthcheck or to an upstream load balancer. A
		// single global limiter — the obvious "simplification" of four per-route
		// ones — breaks exactly this and nothing else.
		await Promise.all(Array.from({ length: 20 }, () => hub.get("/session")));
		await expectStatus(await hub.get("/health"), 200);

		// The authenticated surface is unaffected too: /validate has its own
		// bucket (burst 16), so a client that exhausted /session can still be
		// authorised by nginx.
		await expectStatus(await hub.get("/validate"), 401);
	});
});

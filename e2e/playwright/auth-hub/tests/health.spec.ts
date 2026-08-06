import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { healthSchema } from "../src/schemas.js";

/**
 * `/health` — the port of `01-health.hurl`, plus the two properties that made
 * it usable as a liveness probe and that nothing asserted.
 *
 * auth-hub sits in front of every authenticated request into the platform, so
 * its health route is what compose's own healthcheck (`/auth-hub healthcheck`,
 * which GETs this path on 127.0.0.1) and any upstream load balancer poll. Two
 * things have to hold for that to work at all: it must need no credentials,
 * and it must not be rate limited. Both are properties of `main.go`'s route
 * table rather than of the handler, which is why the handler's unit test
 * cannot see either of them.
 */

test.describe("health", () => {
	test("GET /health reports healthy @smoke", async ({ hub }) => {
		// `healthSchema` is strict(). auth-hub ships two health handlers and only
		// `internal/adapter/handler/health.go` is the one main.go mounts; the
		// legacy `handler/health_handler.go` adds a `service` field. An extra key
		// therefore means the wrong handler got wired — see src/schemas.ts.
		await expectJsonStatus(await hub.get("/health"), 200, healthSchema);
	});

	test("GET /health needs no session cookie @smoke", async ({ hub }) => {
		// `hub` is anonymous by construction; restating the claim as its own test
		// keeps it alive if the fixture ever grows a default credential.
		await expectStatus(await hub.get("/health"), 200);
	});

	test("GET /health is not behind a rate limiter @contract", async ({ hub }) => {
		// main.go:180 registers `/health` with no `RateLimiter.Middleware()`,
		// unlike /validate, /session, /csrf and the /internal group. That absence
		// is load-bearing: the container healthcheck polls every 3s
		// (compose.staging.yaml), and a limiter here would make a restarting
		// service look permanently unhealthy and never converge.
		//
		// 25 back-to-back requests is well past every configured burst in the
		// service (the largest non-CSRF one is /validate at 16), so a single 429
		// in this loop means a limiter was attached to the wrong route.
		const statuses: number[] = [];
		for (let i = 0; i < 25; i += 1) {
			statuses.push((await hub.get("/health")).status());
		}
		expect(statuses.filter((status) => status !== 200)).toEqual([]);
	});

	test("POST /health is not routed @contract", async ({ hub }) => {
		// `e.GET("/health", ...)`. Echo answers a known path with an unregistered
		// verb as 405, not 404 — which is how this suite tells "the route table
		// has this path" apart from "it does not" everywhere else it makes that
		// distinction (tests/topology.spec.ts).
		await expectStatus(await hub.post("/health"), 405);
	});
});

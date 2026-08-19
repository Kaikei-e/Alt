import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { csrfTokenSchema, healthSchema } from "../src/schemas.js";

/**
 * Public surface smoke — the port of `01-health-csrf.hurl`.
 *
 * `/metrics` is asserted on the ops listener (:9110), not the REST port. The
 * three-binary split (ADR-000954) gives alt-backend / alt-harvester /
 * alt-data-hub one common operator listener, and
 * observability/prometheus/prometheus.yml scrapes all three there. A
 * Prometheus endpoint on the browser-facing listener was only ever
 * incidental; tests/topology.spec.ts asserts it is gone from :9000.
 */
test.describe("public surface", () => {
	test("GET /v1/health reports healthy", async ({ restAnon }) => {
		const response = await restAnon.get("/v1/health");
		await expectJsonStatus(response, 200, healthSchema);
		expectHeaderContains(response, "Cache-Control", "max-age=30");
	});

	test("GET /v1/health is public — no JWT required", async ({ restAnon }) => {
		// The DoS whitelist exempts /v1/health from rate limiting; the auth
		// middleware exempts it from the JWT requirement. Both are what lets
		// compose's healthcheck and Prometheus blackbox probes work at all.
		const response = await restAnon.get("/v1/health");
		await expectStatus(response, 200);
	});

	test("GET /v1/csrf-token mints a token without authentication", async ({ restAnon }) => {
		const body = await expectJsonStatus(
			await restAnon.get("/v1/csrf-token"),
			200,
			csrfTokenSchema,
		);
		expect(body.csrf_token.length).toBeGreaterThanOrEqual(32);
	});

	test("GET /v1/csrf-token mints a distinct token per call", async ({ restAnon }) => {
		// The driver stores each token in a sync.Map keyed by its own value, so
		// two calls handing back the same string would mean the generator lost
		// its entropy — the one failure mode that turns CSRF into decoration.
		const [first, second] = await Promise.all([
			restAnon.get("/v1/csrf-token"),
			restAnon.get("/v1/csrf-token"),
		]);
		const a = await expectJsonStatus(first, 200, csrfTokenSchema);
		const b = await expectJsonStatus(second, 200, csrfTokenSchema);
		expect(a.csrf_token).not.toBe(b.csrf_token);
	});

	test("GET /metrics on the ops listener serves Prometheus text", async ({ ops }) => {
		const response = await ops.get("/metrics");
		await expectStatus(response, 200);
		expectHeaderContains(response, "Content-Type", "text/plain");
		expect(await response.text()).toContain("# HELP");
	});

	test("GET /health on the ops listener answers the operator probe", async ({ ops }) => {
		// `healthcheck` (the container's own subcommand) self-probes this. It is
		// a different route from the public /v1/health and must not be confused
		// with it — tests/topology.spec.ts pins that neither leaks onto :9000.
		await expectStatus(await ops.get("/health"), 200);
	});
});

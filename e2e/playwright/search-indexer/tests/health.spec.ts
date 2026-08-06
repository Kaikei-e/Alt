import { expect, test } from "../src/fixtures.js";
import { expectHeader, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { env } from "../src/env.js";
import { connectHealthSchema, restHealthSchema } from "../src/schemas.js";

/**
 * Liveness on both plaintext listeners — the port of `01-health.hurl`, plus
 * the half of it the Hurl suite never had.
 *
 * search-indexer serves *two* `/health` routes, from two different
 * `http.Server` instances started in two different goroutines, and they answer
 * deliberately different bodies:
 *
 *   :9300  `{"status":"ok"}`                              bootstrap/servers.go
 *   :9301  `{"status":"healthy","service":"connect-rpc"}` connect/v2/server.go
 *
 * The container healthcheck (`/search-indexer healthcheck`, main.go) only ever
 * probes the first, over 127.0.0.1 — so a Connect listener that failed to bind
 * leaves the container reporting healthy and every RPC caller getting
 * connection refused. That is exactly the CLAUDE.md rule 8 shape ("a
 * dependency that did not come up is indistinguishable from one that is
 * intentionally off"), and it is why the second assertion below exists.
 */
test.describe("liveness", () => {
	test("REST :9300 GET /health reports ok", { tag: "@smoke" }, async ({ rest }) => {
		const response = await rest.get("/health");
		await expectJsonStatus(response, 200, restHealthSchema);
		// `Set("Content-Type", "application/json")` — no charset parameter,
		// unlike the search handler two routes over, which sets
		// `application/json; charset=utf-8`. Asserting the exact value rather
		// than `contains` (which is all Hurl could express) keeps the two from
		// silently converging, which is the first thing that happens when
		// someone "tidies up" the mux.
		expectHeader(response, "Content-Type", "application/json");
	});

	test(
		"Connect :9301 GET /health reports the connect-rpc envelope",
		{ tag: "@smoke" },
		async ({ bare }) => {
			// New coverage: `e2e/hurl/search-indexer/README.md` put :9301 under
			// "out of scope — HTTP/2 h2c, not practical to drive from Hurl". The
			// route is a plain `mux.HandleFunc` registered *outside* the Connect
			// handler and outside its interceptor chain (connect/v2/server.go),
			// so it is the cheapest possible proof that the second server bound
			// its port, independent of whether any procedure is registered.
			const response = await bare.get(`${env.connectURL}/health`);
			await expectJsonStatus(response, 200, connectHealthSchema);
			expectHeader(response, "Content-Type", "application/json");
		},
	);

	test(
		"the two /health routes are distinct handlers, not one mux served twice",
		{ tag: "@contract" },
		async ({ rest, bare }) => {
			// The bodies differ by construction. If they ever match, either the
			// muxes were merged — which would also put `/v1/search` on the
			// Connect port, and tests/topology.spec.ts asserts it is not there —
			// or one listener is proxying the other, in which case every
			// topology assertion in this suite is proving nothing.
			const restBody = await expectJsonStatus(await rest.get("/health"), 200, restHealthSchema);
			const connectBody = await expectJsonStatus(
				await bare.get(`${env.connectURL}/health`),
				200,
				connectHealthSchema,
			);
			expect(restBody.status).not.toBe(connectBody.status);
		},
	);

	test("health is not gated behind any credential", { tag: "@authz" }, async ({ bare }) => {
		// Stated as a test rather than left implicit: this is what makes the
		// compose healthcheck and any blackbox probe work at all, and it is the
		// baseline the `/v1/search` posture assertion in topology.spec.ts is
		// measured against. `bare` sends no headers whatsoever.
		await expectStatus(await bare.get(`${env.baseURL}/health`), 200);
	});
});

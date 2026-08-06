import { test, expect } from "../src/fixtures.js";
import {
	expectHeaderContains,
	expectJsonStatus,
	expectPrometheusText,
	expectStatus,
} from "../../_shared/http.js";
import { connectHealthSchema, opsHealthSchema } from "../src/schemas.js";

/**
 * The two health surfaces — the port of `00-setup.hurl`'s assertions and the
 * top half of `03-ops-listener.hurl`.
 *
 * `00-setup.hurl` itself does not survive as a test: its `retry: 60` was a
 * readiness gate, and that moved to `setup/global-setup.ts` (see the header
 * there). What survives is what it *asserted* once it stopped retrying — the
 * body of `/health` on the mTLS listener — which is a contract, not a gate,
 * and belongs in a spec that can fail on its own.
 *
 * The pair matters more than either half. Both routes answer
 * `{"status":"healthy","service":...}` and they differ only in the service
 * name, so a slice that brought up the wrong binary on the `alt-data-hub`
 * name, or a refactor that mounted `NewOpsHandler` behind the mTLS listener,
 * changes exactly one string. Pinning both literals is what notices.
 */

test.describe("health", () => {
	test("mTLS /health names the Connect surface", { tag: "@smoke" }, async ({ dataHub }) => {
		// `connect-rpc`, not `alt-data-hub`: connect/v2/muxutil.RegisterHealth
		// mounts this on every Connect listener in the module and writes a
		// fixed body naming the *surface*. The same bytes come back from
		// alt-backend's :9101 and :9102. The per-binary name is on :9110.
		const response = await dataHub.get("/health");
		await expectJsonStatus(response, 200, connectHealthSchema);
		expectHeaderContains(response, "Content-Type", "application/json");
	});

	test("ops /health names this binary", { tag: "@smoke" }, async ({ ops }) => {
		// `internal/bootstrap.NewOpsHandler(serviceName, ...)` is constructed
		// with cmd/datahub's `serviceName` constant, so this string is the one
		// thing on the operator surface that says *which* of the three split
		// binaries is answering. Everything else about :9110 is identical
		// across alt-backend, alt-harvester and alt-data-hub.
		await expectJsonStatus(await ops.get("/health"), 200, opsHealthSchema);
	});

	test("ops /metrics serves Prometheus exposition", { tag: "@smoke" }, async ({ ops }) => {
		// A 503 here would be `NewOpsHandler`'s explicit unwired branch: "the
		// route exists and its dependency does not" (OTel returned no
		// Prometheus handler). That is CLAUDE.md rule 8's visible half, and
		// `expectPrometheusText` fails it because it demands 200 — which is the
		// point. A silently blank scrape target is the failure mode this
		// endpoint exists to make impossible.
		//
		// No metric family is named. The exposition here comes from the OTel
		// Prometheus exporter with no Alt-specific instrumentation registered
		// on this binary, so naming a family would pin an upstream library's
		// output rather than anything alt-data-hub promises.
		const body = await expectPrometheusText(await ops.get("/metrics"));
		expect(body, "an exposition with HELP lines but no TYPE lines is truncated").toContain(
			"# TYPE",
		);
	});

	test("ops /health needs no credential", { tag: "@authz" }, async ({ ops }) => {
		// The bargain the plaintext operator listener rests on: Prometheus
		// scrapes alt-data-hub over alt-network, and giving it a client
		// certificate would mean putting a monitoring identity into
		// DATAHUB_ALLOWED_PEERS — diluting the data plane's entire
		// authorisation decision to buy a gauge (cmd/datahub/main.go). The
		// `ops` fixture carries no certificate and no token, so a 200 here is
		// that bargain holding. tests/topology.spec.ts asserts the other half:
		// that nothing else is reachable through this door.
		await expectStatus(await ops.get("/health"), 200);
	});
});

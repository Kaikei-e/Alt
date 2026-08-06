import { test } from "../src/fixtures.js";
import {
	expectHeaderContains,
	expectJsonStatus,
	expectPrometheusText,
} from "../../_shared/http.js";
import { connectHealthSchema, healthzSchema, readyzSchema } from "../src/schemas.js";

/**
 * The three health surfaces, plus `/metrics` — the port of `00-readiness.hurl`,
 * `01-healthz.hurl` and `02-connect-health.hurl`.
 *
 * They stay as tests even though `setup/global-setup.ts` already waits on all
 * three, and the duplication is deliberate: the gate proves the stack *came
 * up*, these prove the **bodies** are still what an operator and a compose
 * healthcheck depend on. A `/readyz` that started answering
 * `{"ok": true}` would sail through a gate that only checks `response.ok()`
 * and break every probe in production.
 */

test.describe("REST health surface", () => {
	test("GET /healthz is a static liveness answer", { tag: "@smoke" }, async ({ rest }) => {
		// main.go:125 — the handler closes over nothing and touches no
		// dependency. That is the contract: liveness must stay green while the
		// database is down, or Kubernetes-style restarts would cascade on a DB
		// blip. `/readyz` below is the one that is allowed to go red.
		const response = await rest.get("/healthz");
		await expectJsonStatus(response, 200, healthzSchema);
		expectHeaderContains(response, "Content-Type", "application/json");
	});

	test("GET /readyz reports the DB pool is live", { tag: "@smoke" }, async ({ rest }) => {
		// main.go:128 calls `dbPool.Ping`. The literal `"ready"` matters: the
		// failure branch answers 503 `{"status":"db down"}` with the same key,
		// so asserting only the key would accept the outage.
		const response = await rest.get("/readyz");
		await expectJsonStatus(response, 200, readyzSchema);
		expectHeaderContains(response, "Content-Type", "application/json");
	});

	test(
		"GET /metrics publishes Prometheus exposition text",
		{ tag: "@contract" },
		async ({ rest }) => {
			// New coverage: the Hurl suite never touched `/metrics`, so a
			// `promhttp` mount lost in a refactor (main.go:141) would have taken
			// the service's whole observability surface with it and nothing would
			// have gone red.
			//
			// The two families named are the Go collector's, which client_golang
			// registers on `DefaultRegisterer` in its own package init — so they
			// are published by construction, and their absence means `/metrics`
			// is being served by something other than the exporter this stack
			// thinks it is. `promhttp_metric_handler_*` is deliberately NOT
			// among them: main.go:141 mounts the bare `promhttp.Handler()`, not
			// `InstrumentMetricHandler`, so the handler's own instrumentation
			// (which the alt-harvester suite can assert) does not exist here.
			//
			// The service-specific family —
			// `rag_orchestrator_knowledge_event_emitter_failure_total` — is also
			// left out on purpose: it is a CounterVec
			// (adapter/sovereign_client/metrics.go:23), and a labelled vector
			// emits no series until a label combination is used, so requiring it
			// would mean requiring an emit failure to have happened first.
			await expectPrometheusText(await rest.get("/metrics"), [
				"go_goroutines",
				"go_info",
			]);
		},
	);
});

test.describe("Connect-RPC health surface", () => {
	test(
		"GET /connect/health identifies the Connect listener",
		{ tag: "@smoke" },
		async ({ connect }) => {
			// connect/server.go:76 registers this on the Connect mux itself,
			// independent of the Augur and MorningLetter handlers — which is what
			// makes it useful: it proves the second `http.Server` (main.go:171) is
			// serving even when every RPC below it is failing.
			//
			// `service: "connect-rpc"` is asserted, not just `status`: the two
			// listeners both answer JSON `{"status": ...}`, and this field is the
			// only thing in either body that says *which* one replied.
			//
			// It also pins the slice's `PEER_IDENTITY_MODE=disabled`
			// configuration by its visible consequence: this is a *cleartext*
			// GET. Flip compose.staging.yaml to `mtls` and main.go:166 swaps in
			// `CreateMTLSConnectServer`, which terminates TLS on the same port —
			// this call would then fail at the transport instead of returning a
			// body, and say so.
			const response = await connect.get("/connect/health");
			await expectJsonStatus(response, 200, connectHealthSchema);
			expectHeaderContains(response, "Content-Type", "application/json");
		},
	);
});

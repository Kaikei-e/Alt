import { test, expect } from "../src/fixtures.js";
import {
	expectHeaderContains,
	expectJsonStatus,
	expectPrometheusText,
} from "../../_shared/http.js";
import { eventually } from "../../_shared/eventual.js";
import { healthSchema } from "../src/schemas.js";

/**
 * Health and observability on both listeners.
 *
 * Ports `00-setup.hurl` (RPC-port health) and `01-health-metrics-port.hurl`
 * (operator-port health) and adds the `/metrics` surface, which the Hurl suite
 * never touched at all despite the port being named after it.
 */

test.describe("health", () => {
	test("GET /health on the RPC listener reports ok", { tag: "@smoke" }, async ({ rpc }) => {
		// `00-setup.hurl` asserted `$.status == "ok"` and `$.service ==
		// "knowledge-sovereign"` as two separate JSONPath checks. The schema
		// asserts the same two as literals in one step, so a handler that
		// starts answering `{"status":"degraded"}` fails on the field that
		// changed rather than on whichever check happened to run first.
		const response = await rpc.get("/health");
		await expectJsonStatus(response, 200, healthSchema);
		expectHeaderContains(response, "Content-Type", "application/json");
	});

	test("GET /health on the operator listener reports ok", { tag: "@smoke" }, async ({ admin }) => {
		// This is the port compose's healthcheck probes
		// (`wget --spider http://127.0.0.1:9501/health`), so it is the one a
		// deployment's readiness actually depends on.
		const response = await admin.get("/health");
		await expectJsonStatus(response, 200, healthSchema);
		expectHeaderContains(response, "Content-Type", "application/json");
	});

	test(
		"both listeners serve the identical health document",
		{ tag: "@contract" },
		async ({ rpc, admin }) => {
			// main.go registers the *same* `handler.HealthHandler` on both
			// muxes. `01-health-metrics-port.hurl` described its own job as
			// "must return the same schema as :9500" but could only re-assert
			// the two fields independently — two handlers that had drifted
			// apart would both still pass. Comparing the documents is the
			// assertion that description meant.
			const [fromRpc, fromAdmin] = await Promise.all([
				rpc.get("/health"),
				admin.get("/health"),
			]);
			const a = await expectJsonStatus(fromRpc, 200, healthSchema);
			const b = await expectJsonStatus(fromAdmin, 200, healthSchema);
			expect(a).toEqual(b);
		},
	);

	test(
		"GET /health is unauthenticated on the operator listener",
		{ tag: "@authz" },
		async ({ admin }) => {
			// `requireAdminToken` in main.go gates only paths under `/admin/`,
			// and deliberately so: the compose healthcheck and Prometheus
			// blackbox probes carry no token. If a future change moved the gate
			// up to the whole mux, every deployment would fail its healthcheck
			// — this is the fence.
			const response = await admin.get("/health", { headers: { Authorization: "" } });
			await expectJsonStatus(response, 200, healthSchema);
		},
	);
});

test.describe("metrics", () => {
	test("GET /metrics serves Prometheus exposition", { tag: "@smoke" }, async ({ admin }) => {
		// New coverage. `promhttp.Handler()` on the default registry, which
		// carries the process and Go runtime collectors out of the box. A
		// `/metrics` that answers 200 with an empty body is the classic silent
		// observability failure — the scrape succeeds, the dashboards stay
		// blank, nothing alerts — so the families are named rather than merely
		// counted.
		await expectPrometheusText(await admin.get("/metrics"), [
			"go_goroutines",
			"process_start_time_seconds",
		]);
	});

	test(
		"the projection-health exporter publishes its liveness gauge",
		{ tag: "@slow" },
		async ({ admin }) => {
			// New coverage, and the reason it is worth the wall clock:
			// `knowledge_event_last_occurrence_age_seconds` is registered with
			// `promauto` at package init, but a GaugeVec publishes **no series
			// at all** until something calls `WithLabelValues(...).Set(...)`.
			// That only happens inside `projection_health.Exporter.RunOnce`,
			// which only runs from the ticker goroutine main.go starts.
			//
			// So the family appearing is the one externally observable proof
			// that the exporter goroutine was actually wired — CLAUDE.md rule 8
			// at the boundary. Delete the `go func()` in main.go and every unit
			// test still passes, `/metrics` still answers 200, and the
			// recap-dead alert silently never fires again.
			//
			// The tick is KNOWLEDGE_SOVEREIGN_PROJECTION_HEALTH_TICK_INTERVAL,
			// unset in staging, so config.Load's 60s default applies and the
			// first sample lands up to a minute after boot. Usually it has
			// already happened by the time the suite runs (installing
			// dependencies alone takes longer); `eventually` covers the case
			// where it has not, without a sleep a fast stack would pay for.
			test.setTimeout(150_000);
			await eventually(
				async () => {
					const body = await expectPrometheusText(await admin.get("/metrics"));
					expect(body).toContain("knowledge_event_last_occurrence_age_seconds");
					// One series per entry in projection_health.WatchedEventTypes.
					// Naming them individually catches a truncated loop, which a
					// bare family-name check would not.
					for (const eventType of [
						"recap.topic_snapshotted.v1",
						"SummarySuperseded",
						"TagSetVersionCreated",
						"SummaryVersionCreated",
					]) {
						expect(body).toContain(`event_type="${eventType}"`);
					}
				},
				{
					timeout: 120_000,
					intervals: [1_000, 2_000, 5_000],
					message:
						"projection_health.Exporter.RunOnce publishes knowledge_event_last_occurrence_age_seconds",
				},
			);
		},
	);
});

import { test, expect } from "../src/fixtures.js";
import {
	expectHeaderContains,
	expectJsonStatus,
	expectPrometheusText,
	expectStatus,
} from "../../_shared/http.js";
import { expectExpositionLines } from "../src/assertions.js";
import { healthLiveSchema, healthReadySchema } from "../src/schemas.js";

/**
 * The operator surface — the port of `00-setup.hurl`, `01-health-schema.hurl`
 * and `02-metrics-shape.hurl`.
 *
 * The two health probes are deliberately different animals and the suite
 * depends on the difference:
 *
 *   /health/live   dependency-free. Nothing but a tracing call
 *                  (api/health.rs:55-61). This is what a liveness probe must
 *                  be — a restart triggered by an upstream being slow is how a
 *                  dependency outage becomes a crash loop.
 *   /health/ready  pings recap-subworker and news-creator in sequence and
 *                  turns either failure into 503 + `{"status":"degraded"}`
 *                  (api/health.rs:31-53).
 *
 * `setup/global-setup.ts` gates the run on both, which makes these two tests
 * look redundant. They are not, and the duplication is the same one
 * alt-backend's health.spec.ts carries: the gate's job is to fail *once* with
 * a legible message when the stack never comes up, and it asserts only the
 * `status` field it needs for that. These assert the envelope — including the
 * field that must **not** be there — and would survive the gate being
 * restructured.
 */

test.describe("health probes", () => {
	test("GET /health/live answers 200 with a bare live envelope @smoke @contract", async ({
		api,
	}) => {
		const response = await api.get("/health/live");
		// The schema is `.strict()`. `HealthReport` shares one struct between
		// the live and degraded paths, with `detail` suppressed by
		// `skip_serializing_if = "Option::is_none"` (api/health.rs:7-13). A live
		// response that grew a `detail` key would mean the dependency-free probe
		// had started reporting on a dependency — which is exactly what makes it
		// unusable as the liveness probe compose points at.
		await expectJsonStatus(response, 200, healthLiveSchema);
		expectHeaderContains(response, "Content-Type", "application/json");
	});

	test("GET /health/ready answers 200 once both upstreams answer @smoke @contract", async ({
		api,
	}) => {
		// A 503 here means one of `subworker.ping()` / `news_creator
		// .health_check()` failed, and the body names which — so the assertion
		// failure carries its own diagnosis via expectJsonStatus's body preview.
		const response = await api.get("/health/ready");
		await expectJsonStatus(response, 200, healthReadySchema);
		expectHeaderContains(response, "Content-Type", "application/json");
	});

	test("/health/live and /health/ready are distinct routes @contract", async ({ api }) => {
		// The two are separate `.route(...)` lines (api.rs:24-25) with different
		// handlers, and the status strings are the only thing distinguishing
		// them on the wire. A refactor that pointed both at `health::live`
		// would make the readiness gate — and compose's dependency ordering —
		// silently unconditional, while every "is it 200" check kept passing.
		const [live, ready] = await Promise.all([
			api.get("/health/live"),
			api.get("/health/ready"),
		]);
		const liveBody = await expectJsonStatus(live, 200, healthLiveSchema);
		const readyBody = await expectJsonStatus(ready, 200, healthReadySchema);
		expect(liveBody.status).not.toBe(readyBody.status);
	});
});

test.describe("metrics", () => {
	test("GET /metrics publishes the families this service owes @smoke @contract", async ({
		api,
	}) => {
		// Port of 02-metrics-shape.hurl, which asserted 200 + text/plain and
		// stopped there because recap-worker's exporter was mute: it gathered
		// the process-wide default registry while every instrument was
		// registered into one the service owned. The endpoint answered 200 with
		// an empty body, so the scrape succeeded and the dashboards stayed
		// blank.
		//
		// The families are named rather than counted. A count passes against
		// any exporter that publishes something, and the failure mode worth
		// catching is narrower than "publishes nothing": an instrument moved to
		// a second registry, or a module dropped from the build, takes its own
		// family with it and leaves the rest of the exposition intact.
		//
		// The two relay gauges lead the list because they are read from outside
		// this service — observability/prometheus/rules/push-delivery-alerts.yml
		// names recap-worker as a source for them, and an alert whose guard
		// evaluates over an absent series does not fire.
		const response = await api.get("/metrics");
		expectHeaderContains(response, "Content-Type", "text/plain");
		const body = await expectPrometheusText(response, [
			"notification_outbox_last_tick_timestamp_seconds",
			"notification_outbox_oldest_pending_age_seconds",
			"recap_jobs_completed_total",
			"recap_jobs_failed_total",
			"recap_articles_fetched_total",
			"recap_clusters_created_total",
			"recap_summaries_generated_total",
			"recap_active_jobs",
		]);
		expectExpositionLines(body, response.url());
	});

	test("GET /metrics ignores a caller credential rather than rejecting it @authz", async ({
		api,
	}) => {
		// Not a gap — a fact worth pinning. The staging slice runs
		// `PEER_IDENTITY_TRUSTED=off` and `api::router` installs no auth layer
		// at all (api.rs:17-70), so /metrics is reachable by anyone who can
		// reach the port. That is only safe because the port never faces the
		// internet; tests/topology.spec.ts asserts the other listener is closed.
		//
		// The request carries a credential the service has no way to validate,
		// which is what makes this something other than the test above run
		// twice: a bearer token must be *inert* here, not a 401 and not a 403.
		// The day an auth layer is added, this is where the decision to exempt
		// (or not exempt) the scrape path gets made explicitly — the same shape
		// tests/topology.spec.ts uses for X-Alt-Peer-Identity on the dashboard.
		const response = await api.get("/metrics", {
			headers: { Authorization: "Bearer definitely-invalid" },
		});
		await expectStatus(response, 200);
	});
});

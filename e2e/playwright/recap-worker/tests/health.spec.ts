import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { expectPrometheusExposition } from "../src/assertions.js";
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
	test("GET /metrics serves Prometheus exposition text @smoke @contract", async ({ api }) => {
		// Port of 02-metrics-shape.hurl, which asserted 200 + text/plain and
		// stopped there because recap-worker's exporter is currently mute. The
		// reason is a real registry-binding bug, spelled out in
		// src/assertions.ts — `_shared/http.ts`'s `expectPrometheusText` would
		// fail here, and pinning the bug in place with a "body is empty"
		// assertion would be worse than useless.
		//
		// What is asserted instead survives the fix: the envelope the scraper
		// depends on, plus the syntax of whatever lines are present. The day the
		// families reach the default registry this still passes, and the
		// family-name assertion belongs in that change.
		const response = await api.get("/metrics");
		await expectStatus(response, 200);
		expectHeaderContains(response, "Content-Type", "text/plain");
		expectPrometheusExposition(await response.text(), response.url());
	});

	test("GET /metrics needs no authentication @authz", async ({ api }) => {
		// Not a gap — a fact worth pinning. The staging slice runs
		// `PEER_IDENTITY_TRUSTED=off` and `api::router` installs no auth layer
		// at all (api.rs:17-70), so /metrics is reachable by anyone who can
		// reach the port. That is only safe because the port never faces the
		// internet; tests/topology.spec.ts asserts the other listener is closed.
		// If an auth layer is ever added, this test is where the decision to
		// exempt (or not exempt) the scrape path gets made explicitly.
		await expectStatus(await api.get("/metrics"), 200);
	});
});

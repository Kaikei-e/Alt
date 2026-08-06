import { test, expect } from "../src/fixtures.js";
import {
	expectHeader,
	expectHeaderContains,
	expectJsonStatus,
	expectNoHeader,
	expectPrometheusText,
	expectStatus,
} from "../../_shared/http.js";
import { opsHealthSchema, opsHealthSmokeSchema } from "../src/schemas.js";
import { requirePrometheusSample } from "../src/surface.js";

/**
 * The two surfaces alt-harvester actually serves — the positive half of
 * `01-operator-surface-only.hurl`.
 *
 * `bootstrap.NewOpsHandler` mounts exactly `/health` and `/metrics` on an
 * explicit `http.ServeMux`, with no middleware, no authentication and no
 * versioned namespace. `/health` is what the compose probe and
 * `bootstrap.Healthcheck` read; `/metrics` is what the `alt-harvester` scrape
 * job in observability/prometheus/prometheus.yml reads.
 *
 * These are what give tests/absent-surfaces.spec.ts its meaning. Sixty 404
 * assertions against a container that is not running would all pass; the
 * assertions below are the control that says the listener is there and
 * answering.
 */

test.describe("the operator listener answers", () => {
	test("GET /health reports healthy", { tag: "@smoke" }, async ({ ops }) => {
		// The cheapest possible statement that the deployment is not completely
		// broken: this is the same route the container's own healthcheck
		// subcommand probes over loopback, seen from the network side.
		await expectJsonStatus(await ops.get("/health"), 200, opsHealthSmokeSchema);
	});

	test(
		"GET /health names this binary, and pins the whole envelope",
		{ tag: "@contract" },
		async ({ ops }) => {
			// `service` is the only field that distinguishes the three binaries of
			// the split — same Dockerfile, same :9110, same envelope shape — so it
			// is the only assertion that can catch a slice built with the wrong
			// `--build-arg BINARY=`. The schema is strict; see src/schemas.ts for
			// why an extra field on an unauthenticated listener is itself the bug.
			const body = await expectJsonStatus(await ops.get("/health"), 200, opsHealthSchema);
			expect(body.service).toBe("alt-harvester");
		},
	);

	test(
		"GET /health is served as bare application/json",
		{ tag: "@contract" },
		async ({ ops }) => {
			// `w.Header().Set("Content-Type", "application/json")` in
			// internal/bootstrap/ops.go — set explicitly, with no charset and
			// before WriteHeader, so Go's sniffing never runs. The Hurl suite
			// asserted `contains`, which would also have accepted a handler that
			// let net/http sniff the body into `text/plain; charset=utf-8`.
			expectHeader(await ops.get("/health"), "Content-Type", "application/json");
		},
	);

	test(
		"GET /health leaks no server banner",
		{ tag: "@contract" },
		async ({ ops }) => {
			// NewOpsHandler installs no middleware at all, so the only headers on
			// this response are the ones net/http writes. Pinning the absence of a
			// banner is what would catch someone fronting the ops listener with a
			// reverse proxy or an instrumentation wrapper — which for a port whose
			// entire access control is "not published to the host" is a change
			// worth noticing.
			const response = await ops.get("/health");
			expectNoHeader(response, "Server");
			expectNoHeader(response, "X-Powered-By");
		},
	);

	test(
		"POST /health answers too — the ops mux registers no method guard",
		{ tag: "@contract" },
		async ({ ops }) => {
			// `mux.HandleFunc("/health", …)` uses a Go 1.22 pattern with no method,
			// which matches every method. That is harmless today precisely because
			// both routes on this mux are side-effect free reads. It is recorded
			// here so that it is a visible property rather than an accident: the
			// day someone adds a third, mutating route to NewOpsHandler, it will be
			// reachable by GET from anything on the container network, and this is
			// the test whose comment says so.
			await expectStatus(await ops.post("/health", { data: {} }), 200);
		},
	);

	test(
		"GET /metrics serves the Prometheus text exposition format",
		{ tag: ["@smoke", "@contract"] },
		async ({ ops }) => {
			// The families are not decoration. `utils/otel/metrics.go` hands
			// `promhttp.Handler()` — DefaultGatherer, plus the handler's own
			// instrumentation — to the ops mux, and client_golang's package init
			// registers the Go and process collectors on that same registry. So all
			// three below are published by construction, and their absence means
			// the /metrics route is being served by something other than the
			// exporter this stack thinks it is.
			//
			// The Hurl suite asserted only `body contains "# HELP"`, which a single
			// stray HELP line satisfies.
			await expectPrometheusText(await ops.get("/metrics"), [
				"go_goroutines",
				"go_info",
				"promhttp_metric_handler_requests_in_flight",
			]);
		},
	);

	test(
		"GET /metrics is not the unwired-exporter fallback",
		{ tag: "@contract" },
		async ({ ops }) => {
			// NewOpsHandler registers /metrics either way: with the exporter when
			// OTel produced one, and otherwise with a 503 handler that says so in
			// prose. That is CLAUDE.md rule 8 done right — the route exists, the
			// dependency does not, and the two are distinguishable — but it means
			// "the path resolves" proves nothing about observability. Naming the
			// fallback explicitly makes the failure message point at OTEL_ENABLED
			// instead of at Prometheus.
			const response = await ops.get("/metrics");
			const body = await response.text();
			expect(
				response.status(),
				`/metrics answered 503. NewOpsHandler serves that only when ` +
					`OpenTelemetry returned no Prometheus handler — check OTEL_ENABLED ` +
					`and the OTLP endpoint, not the scrape config.\n${body.slice(0, 300)}`,
			).not.toBe(503);
			expect(body).not.toContain("metrics exporter is not wired");
		},
	);

	test(
		"GET /metrics is gathered per scrape, not cached",
		{ tag: "@contract" },
		async ({ ops }) => {
			// promhttp's own request counter is incremented after each response is
			// written, so a second scrape must observe a strictly higher value than
			// the first. A /metrics that answered from a snapshot taken at startup
			// would satisfy every other assertion in this file — right shape, right
			// families, right content type — and give Prometheus a flat line
			// forever.
			//
			// Strictly monotonic even under parallel workers: the counter only ever
			// increases, and this test's own first request has completed before its
			// second one is sent.
			const first = requirePrometheusSample(
				await (await ops.get("/metrics")).text(),
				"promhttp_metric_handler_requests_total",
			);
			const second = requirePrometheusSample(
				await (await ops.get("/metrics")).text(),
				"promhttp_metric_handler_requests_total",
			);
			expect(second).toBeGreaterThan(first);
		},
	);

	test(
		"GET /metrics is unauthenticated, like the scrape job assumes",
		{ tag: "@contract" },
		async ({ ops }) => {
			// Prometheus scrapes alt-harvester over alt-network with no credential
			// of any kind (observability/prometheus/prometheus.yml). The bargain —
			// documented on both this listener and alt-data-hub's — is that the
			// surface carries nothing worth authenticating, which is what the
			// absent-surface suite next door is for. This asserts the other half:
			// no header is required to read it.
			const response = await ops.get("/metrics");
			await expectStatus(response, 200);
			expectHeaderContains(response, "Content-Type", "text/plain");
		},
	);
});

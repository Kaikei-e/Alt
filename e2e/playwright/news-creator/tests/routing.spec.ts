import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectPrometheusText } from "../../_shared/http.js";
import { openApiSchema } from "../src/schemas.js";

/**
 * Route registration and the observability mount — entirely new coverage.
 *
 * This file is the FastAPI counterpart of alt-backend's
 * `connect-surface.spec.ts`, and it exists for the same reason: CLAUDE.md
 * rule 8 at the E2E boundary. main.py builds a `DependencyContainer` and then
 * makes nine `app.include_router` calls. If one of those lines is lost in a
 * refactor — or if a usecase the container builds raises and the router is
 * skipped — the endpoint simply stops existing, and every caller gets a 404
 * that reads like "no data for that request". Unit tests never see the router
 * table, and the per-endpoint specs in this suite each only prove *their own*
 * route is there.
 *
 * `/openapi.json` is FastAPI's own view of what it mounted, so asserting
 * against it turns "these ten routes exist" into a single check that fails
 * loudly and names what is missing.
 */

/**
 * Every route in the service, from the `@router.*` decorators in
 * news_creator/handler/. Path *and* method, because a route registered under
 * the wrong verb is as broken as one that is absent.
 */
const EXPECTED_ROUTES = [
	{ path: "/health", method: "get", owner: "create_health_router" },
	{ path: "/queue/status", method: "get", owner: "create_health_router" },
	{ path: "/api/v1/summarize", method: "post", owner: "create_summarize_router" },
	{ path: "/api/generate", method: "post", owner: "create_generate_router" },
	{ path: "/v1/summary/generate", method: "post", owner: "create_recap_summary_router" },
	{ path: "/v1/summary/generate/batch", method: "post", owner: "create_recap_summary_router" },
	{ path: "/api/v1/expand-query", method: "post", owner: "create_expand_query_router" },
	{ path: "/api/v1/plan-query", method: "post", owner: "create_plan_query_router" },
	{ path: "/v1/rerank", method: "post", owner: "create_rerank_router" },
	{ path: "/api/chat", method: "post", owner: "create_chat_router" },
	{ path: "/v1/morning-letter/generate", method: "post", owner: "create_morning_letter_router" },
] as const;

test.describe("route registration", () => {
	test("every router main.py includes is present in the OpenAPI document @smoke @contract", async ({
		api,
	}) => {
		const document = await expectJsonStatus(await api.get("/openapi.json"), 200, openApiSchema);
		const paths = document.paths;

		for (const route of EXPECTED_ROUTES) {
			const operations = paths[route.path];
			expect(
				operations,
				`${route.path} is absent from /openapi.json — ${route.owner} was never ` +
					`included in main.py, or the DependencyContainer failed to build the ` +
					`usecase it needs. This is the failure mode where a caller gets a 404 ` +
					`that reads like "no data" (CLAUDE.md rule 8).`,
			).toBeDefined();
			expect(
				Object.keys(operations ?? {}),
				`${route.path} is mounted but not under ${route.method.toUpperCase()}`,
			).toContain(route.method);
		}
	});

	test("the OpenAPI document identifies this service @contract", async ({ api }) => {
		// `FastAPI(title="News Creator Service", version="2.0.0")` in main.py. A
		// slice that accidentally pointed at a *different* Python service would
		// satisfy most of the shape assertions in this suite — several of the
		// FastAPI services in Alt answer `{"status": "healthy"}` — but not this.
		const document = await expectJsonStatus(await api.get("/openapi.json"), 200, openApiSchema);
		expect(document.info.title).toBe("News Creator Service");
	});
});

test.describe("observability", () => {
	test("GET /metrics serves Prometheus exposition text @smoke @contract", async ({ api }) => {
		// New coverage. main.py does `app.mount("/metrics", make_asgi_app())`,
		// which is a Starlette **mount**, not a route — so it does not appear in
		// /openapi.json and the registration check above cannot see it. A lost
		// mount is the classic silent-observability failure: nothing errors, the
		// scrape 404s, the dashboards go blank and no alert fires because the
		// alerting rules are evaluated on metrics that stopped arriving.
		//
		// The families asserted are prometheus_client's own default collectors,
		// deliberately: the staging slice sets `OTEL_ENABLED=false`, so
		// `init_otel_provider` returns before it constructs the
		// `PrometheusMetricReader` (news_creator/otel.py:56-58) and none of the
		// OTel instruments are registered here. Asserting a
		// `newscreator.*` family would be asserting the staging config, not the
		// service.
		const response = await api.get("/metrics");
		await expectPrometheusText(response, ["python_info"]);
	});
});

import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../src/http.js";
import { healthSchema } from "../src/schemas.js";

/**
 * Topology contract for the alt-backend / alt-harvester / alt-data-hub split
 * — the port of `03-topology-internal-surface-absent.hurl`, from the
 * user-facing binary's side.
 *
 * Two listeners face users, and they must answer exactly what the monolith
 * answered:
 *
 *   :9000   user-facing REST
 *   :9101   user-facing Connect
 *
 * Nothing that authenticates a *service* rather than a user may be reachable
 * on either. The data plane and the `/v1/internal/*` REST routes moved out of
 * this binary entirely, to alt-data-hub behind mTLS, where they are served as
 * `services.datahub.v1.DataHubService` (ADR-000954 Wave 2-C retired
 * `services.backend.v1.BackendInternalService`; ADR-000955 renamed the
 * surviving surface from `alt.datahub.v1`). The admin Connect services do not
 * move — they stay on this binary's loopback-bound operator listener — so for
 * them the claim is narrower but just as load-bearing: not on a port the
 * public NIC can reach.
 *
 * Every negative below asserts **404**, never 401/403. A 401 would mean the
 * routes are still registered on the user-facing mux and only a middleware
 * stands between the public NIC and them; 404 is the only status that says
 * "this surface is not here".
 *
 * The mirror-image assertions — that these same surfaces DO answer on
 * alt-data-hub over mTLS — live in e2e/hurl/alt-data-hub/.
 */

/** Connect procedures that must NOT resolve on the browser-facing mux. */
const ABSENT_FROM_USER_CONNECT = [
	// One procedure per shape of caller: pre-processor's write path, the
	// search-indexer read path, and the recap-worker path that the recap
	// pipeline stub still mounts. Their post-split home is asserted positively
	// in ../../hurl/alt-data-hub/04-datahub-service.hurl.
	"services.datahub.v1.DataHubService/CreateArticle",
	"services.datahub.v1.DataHubService/ListArticlesWithTags",
	"services.datahub.v1.DataHubService/ListRecapArticles",
	// The retired name must stay dead too. Wave 2-C deleted `services.backend.v1`
	// from the proto tree; this is the regression fence against anyone
	// re-adding a compatibility shim on the browser-facing mux.
	"services.backend.v1.BackendInternalService/CreateArticle",
	// The admin Connect surface stays with this binary, on the loopback-bound
	// operator listener. Neither service authenticates its caller, so the bind
	// address is the entire access control — which is precisely why it must not
	// also answer on a port that faces the browser.
	//
	// AdminMonitorService is deliberately not probed: it registers only when
	// config.AdminMonitor.Enabled, which staging leaves false, so a 404 would
	// be the config talking rather than the topology.
	"alt.knowledge_home.v1.KnowledgeHomeAdminService/EmitArticleUrlBackfill",
	"alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth",
] as const;

/** `/v1/internal/*` REST routes that left with cmd/datahub. */
const ABSENT_INTERNAL_REST = [
	"/v1/internal/system-user",
	"/v1/internal/articles/recent",
	"/v1/internal/articles/recent?within_hours=24&limit=10",
] as const;

test.describe("user-facing surface is unchanged by the split", () => {
	test("GET /v1/health still answers on :9000", async ({ restAnon }) => {
		// tests/health.spec.ts smoke-tests this as a public endpoint. The
		// duplication is intentional and has a different lifetime: this
		// assertion is the "the split did not move the user's API" clause and
		// must survive even if the public smoke file is restructured.
		await expectJsonStatus(await restAnon.get("/v1/health"), 200, healthSchema);
	});

	test("a registered Connect procedure still answers on :9101", async ({ connectAnon, connect }) => {
		// This is the *control* that gives the 404 assertions below their
		// meaning. Called without a user JWT, a mounted procedure answers 401
		// (the AuthInterceptor ran, so the handler is there). If an unregistered
		// path and a registered one both answered 404, this file would prove
		// nothing.
		await expectStatus(
			await connectAnon.post("/alt.knowledge_trail.v1.KnowledgeTrailService/GetTrail", {
				data: {},
			}),
			401,
		);

		const authed = await connect.post(
			"/alt.knowledge_trail.v1.KnowledgeTrailService/GetTrail",
			{ data: {} },
		);
		expect(authed.status(), "GetTrail is mounted, so it must not 404").not.toBe(404);
	});
});

test.describe("service-to-service surfaces are absent from the user-facing ports", () => {
	for (const procedure of ABSENT_FROM_USER_CONNECT) {
		test(`Connect :9101 — ${procedure} → 404`, async ({ connect }) => {
			await expectStatus(await connect.post(`/${procedure}`, { data: {} }), 404);
		});
	}

	test("REST :9000 does not proxy Connect procedures either", async ({ rest, csrf }) => {
		// :9000 is an Echo router, not a Connect mux, so a Connect path here can
		// only ever be an unmatched route. Asserting it anyway pins the
		// negative: a future "just proxy the internal RPCs through the REST
		// server" shortcut breaks here.
		//
		// The CSRF token matters: CSRFMiddleware is installed globally and its
		// default branch protects every unexempted POST, so without a token
		// *any* POST to :9000 answers 403 before Echo's router runs — which
		// would make this pass for the wrong reason (403 "blocked by
		// middleware" is not 404 "route absent").
		const response = await rest.post("/services.datahub.v1.DataHubService/CreateArticle", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: {},
		});
		await expectStatus(response, 404);
	});

	for (const path of ABSENT_INTERNAL_REST) {
		test(`REST :9000 — GET ${path} → 404 (unauthenticated)`, async ({ restAnon }) => {
			// Both handlers answered system-level queries with no tenant
			// predicate and carried no auth middleware of their own, so
			// reachability was the entire control. GET is the registered verb, so
			// these skip CSRF and land straight on the router.
			await expectStatus(await restAnon.get(path), 404);
		});

		test(`REST :9000 — GET ${path} → 404 (authenticated)`, async ({ rest }) => {
			// A valid JWT must not resurrect them either: the claim is that the
			// route table has no entry, not that a middleware rejects the caller.
			await expectStatus(await rest.get(path), 404);
		});
	}
});

test.describe("operator surfaces are not on the user-facing ports", () => {
	test("GET /metrics is gone from :9000", async ({ restAnon }) => {
		// /metrics moved to the shared operator listener on :9110 so all three
		// split binaries expose the same shape, and so alt-data-hub can be
		// scraped without handing Prometheus a client certificate
		// (observability/prometheus/prometheus.yml). tests/health.spec.ts
		// asserts the positive on the ops listener; this is the other half.
		await expectStatus(await restAnon.get("/metrics"), 404);
	});

	test("GET /health is gone from :9000", async ({ restAnon }) => {
		// The operator listener's own /health must not leak onto :9000 either.
		// The public health route is /v1/health, asserted above.
		await expectStatus(await restAnon.get("/health"), 404);
	});

	test("the operator Connect surface is not on :9101", async ({ connect }) => {
		// Covered procedure-by-procedure above; this restates the claim at the
		// listener level so the reason survives if the list is ever trimmed.
		const response = await connect.post(
			"/alt.knowledge_home.v1.KnowledgeHomeAdminService/GetFeatureFlags",
			{ data: {} },
		);
		await expectStatus(response, 404);
	});
});

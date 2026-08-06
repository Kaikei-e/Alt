import { test, expect } from "../src/fixtures.js";
import { expectHeaderContains, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { opsHealthSmokeSchema } from "../src/schemas.js";
import { expectPathAbsent, expectProcedureAbsentHere } from "../src/surface.js";

/**
 * alt-harvester serves health and metrics, and nothing else — the negative
 * half of `01-operator-surface-only.hurl`, widened considerably.
 *
 * The split gives each container one job and one reason to accept a request.
 * alt-backend answers users, alt-data-hub answers services holding a
 * certificate, and alt-harvester answers no one — it runs schedules. A job
 * runner that also exposes an API is a job runner someone will eventually call
 * synchronously, and the schedule and the request path then share a process, a
 * pool and a failure mode. This file is the fence around that.
 *
 * Every assertion is **404**, never 401/403. A 401 would mean the handler is
 * compiled in and registered here with only a middleware between it and the
 * caller; 404 says the surface does not exist in this binary — which for
 * alt-harvester is a compile-time fact (`di/container_harvester.go` builds no
 * handler at all, so "a job that starts reaching for one of those fails to
 * compile here rather than silently no-op'ing at runtime").
 *
 * What the Hurl suite probed was a sample: four REST paths, one user Connect
 * procedure, two DataHubService procedures, two admin procedures. A sample is
 * enough to catch "someone mounted the whole router here"; it is not enough to
 * catch "someone mounted one service here". The lists below name every service
 * `connect/v2/server.go` registers on the backend and every REST group
 * `orchestrator/rest/` registers, so a single stray `mux.Handle` fails here.
 */

test.describe("control", () => {
	test(
		"the ops listener answers, so the 404s in this file mean something",
		{ tag: "@smoke" },
		async ({ ops }) => {
			// Without this, every assertion in the file would also pass against a
			// container that never started, a wrong DNS name, or a port with
			// nothing on it. tests/ops-surface.spec.ts states the same fact for a
			// different reason; the duplication is deliberate and has a different
			// lifetime.
			await expectJsonStatus(await ops.get("/health"), 200, opsHealthSmokeSchema);
		},
	);
});

/**
 * REST routes registered by `orchestrator/rest/`, one per group.
 *
 * `/v1/health` matters most: it is the monolith's public health route, and it
 * is one character away from the `/health` this listener does serve. If it
 * answers here, the harvester is running the user-facing Echo router.
 *
 * `/v1/dashboard/jobs` is the pointed one. It is the admin view *of the very
 * jobs this binary runs*, which makes it the single most tempting route to
 * "just add here, since the data is local". It belongs to alt-backend, behind
 * RequireAuth + RequireAdmin, and reading it from a listener that
 * authenticates nobody would hand operational logs to anything on the
 * container network.
 */
const ABSENT_REST_GET = [
	"/v1/health",
	"/v1/csrf-token",
	"/v1/feeds/fetch/list",
	"/v1/feeds/fetch/cursor",
	"/v1/feeds/count/unreads",
	"/v1/feeds/stats",
	"/v1/articles/fetch/cursor",
	"/v1/rss-feed-link/list",
	"/v1/rss-feed-link/export/opml",
	"/v1/morning-letter/updates",
	"/v1/rag/context",
	"/v1/dashboard/jobs",
	"/v1/admin/scraping-domains",
] as const;

/**
 * The POST routes, including the two that do not live under `/v1`.
 *
 * `/security/csp-report` and `/sse/v1/rag/answer` are registered on the Echo
 * root rather than on the versioned group (`security_handlers.go`,
 * `augur_handler.go`), so a probe that only walked `/v1` would miss them —
 * and they are exactly the shape of route someone adds outside the group
 * convention and then forgets about.
 */
const ABSENT_REST_POST = [
	"/security/csp-report",
	"/sse/v1/rag/answer",
	"/v1/images/fetch",
	"/v1/feeds/search",
	"/v1/articles/archive",
] as const;

/**
 * Every Connect service `connect/v2/server.go` mounts on alt-backend's
 * user-facing :9101, named by one procedure each.
 *
 * The Hurl suite probed one of these (KnowledgeTrailService). The rest are new
 * coverage, and they are the reason this list is worth having: mounting *one*
 * service on the harvester's ops mux — "the gateway is already linked in" — is
 * a far more likely mistake than mounting the whole router, and a sample of
 * one cannot see it.
 */
const ABSENT_USER_CONNECT = [
	"alt.feeds.v2.FeedService/GetAllFeeds",
	"alt.articles.v2.ArticleService/FetchArticlesCursor",
	"alt.rss.v2.RSSService/ListRSSFeedLinks",
	"alt.augur.v2.AugurService/ListConversations",
	"alt.morning_letter.v2.MorningLetterReadService/GetLatestLetter",
	"alt.morning_letter.v2.MorningLetterService/StreamChat",
	"alt.recap.v2.RecapService/GetThreeDayRecap",
	"alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome",
	"alt.knowledge_trail.v1.KnowledgeTrailService/GetTrail",
	// GlobalSearchService registers on alt-backend only when `container.Search`
	// is non-nil (connect/v2/server.go guards its mux.Handle), so over there a
	// 404 could be the DI container talking. Here the claim is unconditional:
	// no wiring exists under which the harvester mounts it.
	"alt.search.v2.GlobalSearchService/SearchEverything",
] as const;

/**
 * `services.datahub.v1.DataHubService` procedures the harvester **calls** as a
 * client, chosen because they are the ones it holds a gateway for.
 *
 * That is precisely what makes the shortcut tempting: `di.NewHarvesterComponents`
 * already links `datahubv1connect`, so re-registering the *server* side here
 * costs one line. It would restore exactly the many-surfaces-one-binary shape
 * the split exists to remove, and it would do it on a plaintext port with no
 * peer allowlist in front — while the real DataHubService sits behind mutual
 * TLS on alt-data-hub:9443 with DATAHUB_ALLOWED_PEERS.
 *
 * ClaimOutboxBatch and MarkOutboxProcessed are the sharpest pair: they mutate
 * outbox state, and outbox-worker (this binary's 5-second job) is their only
 * legitimate caller. Reachable here, anything on the container network could
 * mark an event PROCESSED that was never published — a silent append-first
 * violation with no audit trail.
 */
const ABSENT_DATA_PLANE = [
	"services.datahub.v1.DataHubService/CreateArticle",
	"services.datahub.v1.DataHubService/GetLatestArticleTimestamp",
	"services.datahub.v1.DataHubService/ClaimOutboxBatch",
	"services.datahub.v1.DataHubService/MarkOutboxProcessed",
	"services.datahub.v1.DataHubService/PruneOutboxEvents",
	"services.datahub.v1.DataHubService/ListFeedsMissingOgImage",
	"services.datahub.v1.DataHubService/SaveScrapingDomain",
	"services.datahub.v1.DataHubService/GetSystemUser",
	"services.datahub.v1.DataHubService/ListRecentArticles",
] as const;

/**
 * Names that were retired and must stay dead.
 *
 * ADR-000954 Wave 2-C deleted `services.backend.v1` from the proto tree and
 * D6 folded the `/v1/internal/*` REST routes into DataHubService. A
 * compatibility shim is the natural thing to reach for when a caller breaks,
 * and the harvester's "spare" operator port is the natural place to put one.
 */
const ABSENT_RETIRED_CONNECT = [
	"services.backend.v1.BackendInternalService/CreateArticle",
	"services.backend.v1.BackendInternalService/GetSystemUser",
] as const;

const ABSENT_RETIRED_REST = ["/v1/internal/system-user", "/v1/internal/articles/recent"] as const;

/**
 * The admin Connect services, which stay on alt-backend's loopback-bound
 * operator listener.
 *
 * Neither service authenticates its caller — the bind address is the entire
 * access control — which is exactly why "the harvester already has an operator
 * port" must not turn into a second home for them. The harvester's :9110 is
 * bound to every interface in the container's netns so Prometheus can reach
 * it, which is a strictly weaker boundary than loopback.
 */
const ABSENT_ADMIN_CONNECT = [
	"alt.knowledge_home.v1.KnowledgeHomeAdminService/EmitArticleUrlBackfill",
	"alt.knowledge_home.v1.KnowledgeHomeAdminService/GetProjectionHealth",
	"alt.knowledge_home.v1.KnowledgeHomeAdminService/GetFeatureFlags",
	"alt.knowledge_home.v1.KnowledgeHomeAdminService/StartReproject",
	// AdminMonitorService registers on alt-backend only when
	// config.AdminMonitor.Enabled, which staging leaves false — so
	// alt-backend's own topology spec deliberately does not probe it. Here the
	// claim is unconditional and therefore worth making: no flag exists that
	// would mount it on the harvester.
	"alt.admin_monitor.v1.AdminMonitorService/Snapshot",
] as const;

/**
 * net/http's own debug surfaces — new coverage, and the one this listener is
 * most at risk from.
 *
 * `bootstrap.NewOpsHandler` builds an explicit `http.NewServeMux()` and says
 * why in a comment: `http.DefaultServeMux` is where `net/http/pprof` registers
 * itself from an `init()`, so any binary that ever links a profiling package
 * publishes heap and goroutine dumps from whichever mux it reused. The comment
 * is the intent; these probes are the enforcement, and they are what would
 * catch a future `import _ "net/http/pprof"` plus a one-line switch back to
 * the default mux.
 *
 * `/debug/pprof/profile` is deliberately not in this list: if the guard ever
 * failed, that handler blocks for 30 seconds by default and the test would
 * report a timeout rather than "the profiler is exposed".
 */
const ABSENT_DEBUG = [
	"/debug/pprof/",
	"/debug/pprof/heap",
	"/debug/pprof/goroutine",
	"/debug/pprof/cmdline",
	"/debug/vars",
] as const;

test.describe("the user-facing REST API is not in this binary", () => {
	for (const path of ABSENT_REST_GET) {
		test(`GET ${path} → 404`, { tag: "@authz" }, async ({ ops }) => {
			await expectPathAbsent(ops, path);
		});
	}

	for (const path of ABSENT_REST_POST) {
		test(`POST ${path} → 404`, { tag: "@authz" }, async ({ ops }) => {
			// No CSRF token and no JWT, unlike the equivalent probe against
			// alt-backend: there is no middleware on this listener to answer first,
			// so a non-404 here can only mean a route table entry.
			await expectStatus(await ops.post(path, { data: {} }), 404);
		});
	}
});

test.describe("the user-facing Connect services are not mounted here", () => {
	for (const procedure of ABSENT_USER_CONNECT) {
		test(`POST /${procedure} → 404`, { tag: "@authz" }, async ({ ops }) => {
			await expectProcedureAbsentHere(ops, procedure);
		});
	}
});

test.describe("the data plane is not mounted here", () => {
	for (const procedure of ABSENT_DATA_PLANE) {
		test(`POST /${procedure} → 404`, { tag: "@authz" }, async ({ ops }) => {
			await expectProcedureAbsentHere(ops, procedure);
		});
	}
});

test.describe("the retired names stay dead", () => {
	for (const procedure of ABSENT_RETIRED_CONNECT) {
		test(`POST /${procedure} → 404`, { tag: "@authz" }, async ({ ops }) => {
			await expectProcedureAbsentHere(ops, procedure);
		});
	}

	for (const path of ABSENT_RETIRED_REST) {
		test(`GET ${path} → 404`, { tag: "@authz" }, async ({ ops }) => {
			await expectPathAbsent(ops, path);
		});
	}
});

test.describe("the admin Connect surface is not here", () => {
	for (const procedure of ABSENT_ADMIN_CONNECT) {
		test(`POST /${procedure} → 404`, { tag: "@authz" }, async ({ ops }) => {
			await expectProcedureAbsentHere(ops, procedure);
		});
	}
});

test.describe("net/http's debug surfaces are not published", () => {
	for (const path of ABSENT_DEBUG) {
		test(`GET ${path} → 404`, { tag: "@authz" }, async ({ ops }) => {
			await expectPathAbsent(ops, path);
		});
	}
});

test.describe("the mux serves exactly two routes", () => {
	test("GET / → 404", { tag: "@authz" }, async ({ ops }) => {
		// Go 1.22 patterns: `mux.HandleFunc("/health", …)` registers an exact
		// match, not a subtree, and nothing registers "/". A mux with a catch-all
		// would answer here — and a catch-all is how an operator surface quietly
		// becomes a proxy.
		await expectPathAbsent(ops, "/");
	});

	test("GET /health/ → 404 — the pattern is exact, not a subtree", { tag: "@authz" }, async ({
		ops,
	}) => {
		// The trailing-slash form is what would resolve if someone ever changed
		// the pattern to "/health/" to hang sub-routes off it. Pinning it keeps
		// the two-route claim literal.
		await expectPathAbsent(ops, "/health/");
	});

	test("GET /metrics/prometheus → 404", { tag: "@authz" }, async ({ ops }) => {
		// Same claim for the other route: /metrics is a leaf, so Prometheus's
		// scrape path cannot drift to a subpath without this failing.
		await expectPathAbsent(ops, "/metrics/prometheus");
	});
});

test.describe("the 404 comes from Go's ServeMux", () => {
	test(
		"an absent path answers http.NotFound's plain text, not a Connect envelope",
		{ tag: "@contract" },
		async ({ ops }) => {
			// This is what makes every 404 above a *topology* fact rather than a
			// business one. If a Connect handler were mounted and simply did not
			// know the procedure, connect-go would still route through
			// `http.NotFound` — but if any framework, proxy or error handler sat in
			// front of this mux, the body would be JSON. Plain `404 page not found`
			// says the request reached a bare `http.ServeMux` with two routes on it
			// and matched neither.
			//
			// `X-Content-Type-Options: nosniff` comes from `http.Error`, which
			// `NotFoundHandler` calls; it is the cheapest available proof that this
			// is Go's own handler and not a hand-written 404.
			const response = await ops.post("/alt.feeds.v2.FeedService/GetAllFeeds", {
				headers: { "Content-Type": "application/json" },
				data: {},
			});
			await expectStatus(response, 404);
			expectHeaderContains(response, "Content-Type", "text/plain");
			expect(response.headers()["x-content-type-options"]).toBe("nosniff");
			expect(await response.text()).toContain("404 page not found");
		},
	);
});

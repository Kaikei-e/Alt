import { test, expect } from "../src/fixtures.js";
import { expectJson, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { ADMIN_ROUTES, procedure } from "../src/env.js";
import {
	activeProjectionVersionSchema,
	connectErrorSchema,
	storageStatsSchema,
} from "../src/schemas.js";

/**
 * Listener topology — entirely new coverage.
 *
 * `main.go` builds two `http.ServeMux`es and two `http.Server`s:
 *
 *   mainMux    (:9500) → `/health` + the Connect service
 *   metricsMux (:9501) → `/health` + `/metrics` + every `/admin/*` route
 *
 * The split is the *only* access control the operator surface has in this
 * service. `requireAdminToken` wraps `metricsMux` alone, and the staging slice
 * turns even that off (`ADMIN_AUTH=disabled`), so what keeps
 * `POST /admin/snapshots/create` — a route that writes gzip archives to disk
 * and can drop partitions — away from a caller who reached the RPC port is
 * that the route is not registered there.
 *
 * The Hurl suite asserted nothing about this. It called `{{base_url}}` for RPC
 * and `{{metrics_url}}` for admin and never once asked what happened if you
 * swapped them, so a refactor that registered the admin handlers on `mainMux`
 * — one line, entirely plausible while consolidating muxes — would have been
 * invisible.
 *
 * Every negative below asserts **404**, never 401/403. A 401 would mean the
 * route is registered on the wrong mux and only a middleware stands between
 * the caller and it; 404 is the only status that says "this surface is not
 * here".
 */

test.describe("the RPC listener carries only the Connect service and /health", () => {
	test(
		"a registered procedure answers on the RPC port",
		{ tag: "@smoke" },
		async ({ rpc }) => {
			// The *control* that gives every 404 below its meaning. If a
			// registered path and an unregistered one both 404'd, this file
			// would prove nothing. `GetActiveProjectionVersion` is the right
			// control because it takes an empty request and still reaches the
			// database, so a 200 here means the mux resolved *and* the handler
			// ran.
			const response = await rpc.post(procedure("GetActiveProjectionVersion"), { data: {} });
			await expectJsonStatus(response, 200, activeProjectionVersionSchema);
		},
	);

	for (const route of ADMIN_ROUTES) {
		test(`RPC port does not serve ${route.method} ${route.path}`, { tag: "@authz" }, async ({
			rpc,
		}) => {
			// `snapshotHandler.RegisterRoutes(metricsMux)` and friends are
			// called with `metricsMux` only. This is the assertion that keeps
			// it that way.
			const response =
				route.method === "POST"
					? await rpc.post(route.path, { data: {} })
					: await rpc.get(route.path);
			await expectStatus(response, 404);
		});
	}

	test("RPC port does not serve /metrics", { tag: "@authz" }, async ({ rpc }) => {
		// `promhttp.Handler()` is mounted on `metricsMux` only. A Prometheus
		// endpoint on the RPC port would publish this service's internals to
		// anything that can reach the data-plane listener.
		await expectStatus(await rpc.get("/metrics"), 404);
	});

	test("RPC port has no catch-all route", { tag: "@contract" }, async ({ rpc }) => {
		// `mainMux` registers exactly two patterns — `/health` and the Connect
		// service prefix — and no `"/"`. A catch-all added later (a static
		// file server, a debug handler, `net/http/pprof`'s init side effect)
		// would silently start answering here, and every 404 assertion in this
		// file would stop meaning anything.
		await expectStatus(await rpc.get("/"), 404);
		await expectStatus(await rpc.get("/debug/pprof/"), 404);
	});
});

test.describe("the operator listener carries no Connect surface", () => {
	test("operator port serves the admin API", { tag: "@smoke" }, async ({ admin }) => {
		// The control for this describe: the operator port genuinely answers
		// its own routes, so the 404s below are about the *Connect* surface
		// being absent rather than the port being down.
		await expectJsonStatus(await admin.get("/admin/storage/stats"), 200, storageStatsSchema);
	});

	for (const method of [
		"GetActiveProjectionVersion",
		"AppendKnowledgeEvent",
		"GetKnowledgeHomeItems",
	]) {
		test(`operator port does not serve ${method}`, { tag: "@authz" }, async ({ admin }) => {
			// `sovereignv1connect.NewKnowledgeSovereignServiceHandler` is
			// mounted on `mainMux` only. If it ever appeared on `metricsMux`
			// too, the whole knowledge write path would inherit the operator
			// port's access posture — which in staging is "no authentication
			// at all".
			const response = await admin.post(procedure(method), {
				headers: { "Content-Type": "application/json" },
				data: {},
			});
			await expectStatus(response, 404);
		});
	}
});

test.describe("admin routes are method-scoped", () => {
	for (const route of ADMIN_ROUTES) {
		const wrongMethod = route.method === "POST" ? "GET" : "POST";

		test(
			`${wrongMethod} ${route.path} is rejected as a wrong method, not a missing route`,
			{ tag: "@contract" },
			async ({ admin }) => {
				// New coverage. The handlers register Go 1.22 method-aware
				// patterns (`mux.HandleFunc("POST /admin/snapshots/create",
				// ...)`), so `net/http` answers 405 with an `Allow` header for
				// the wrong verb and 404 for an unknown path.
				//
				// The distinction is load-bearing in both directions. A 404
				// here would mean the pattern lost its method prefix, which
				// makes `GET /admin/snapshots/create` *create a snapshot* — a
				// destructive side effect on a verb that crawlers, link
				// checkers and browser prefetch all issue freely. A 405 means
				// the method binding is intact.
				const response =
					wrongMethod === "POST"
						? await admin.post(route.path, { data: {} })
						: await admin.get(route.path);
				await expectStatus(response, 405);
				expect(
					response.headers()["allow"],
					`405 must name the permitted verb so a client can correct itself`,
				).toContain(route.method);
			},
		);
	}

	test("an unknown /admin path is a 404, not a 405", { tag: "@contract" }, async ({ admin }) => {
		// The counterpart that makes the 405s above meaningful: if every
		// `/admin/*` path answered 405, the method assertions would be
		// satisfied by a mux that had lost all its routes.
		await expectStatus(await admin.get("/admin/snapshots/nope"), 404);
	});
});

test.describe("admin authentication posture", () => {
	test(
		"the staging slice serves /admin/* without a Bearer token",
		{ tag: "@authz" },
		async ({ admin }) => {
			// This asserts **current staging configuration**, not desired
			// production behaviour, and it is here so the posture is visible
			// rather than implied by every other admin call quietly working.
			//
			// `config.loadAdminAuth` treats a missing token as a *startup
			// failure*: the only way to run the gate off is the explicit
			// `ADMIN_AUTH=disabled`, which compose.staging.yaml sets so the
			// E2E suite can call `/admin/*` directly. Production supplies
			// `ADMIN_TOKEN_FILE` via compose/sovereign.yaml, where an unset
			// token exits non-zero instead of opening the surface
			// (CLAUDE.md rule 9).
			//
			// A garbage Authorization header proves the gate is genuinely
			// *disabled* rather than accidentally accepting anything: with
			// `AdminAuthEnabled` true, `subtle.ConstantTimeCompare` against
			// this value could not possibly match, so a 200 can only mean the
			// wrapper returned `next` untouched.
			const response = await admin.get("/admin/storage/stats", {
				headers: { Authorization: "Bearer not-the-configured-admin-token" },
			});
			await expectJsonStatus(response, 200, storageStatsSchema);
		},
	);
});

test.describe("Connect mux boundaries", () => {
	test(
		"an unknown procedure on the mounted service 404s from Go's mux, not the Connect layer",
		{ tag: "@contract" },
		async ({ rpc }) => {
			// connect-go's generated handler routes only the procedures it
			// knows and hands anything else to `http.NotFound`, so an unknown
			// *procedure* on a known *service* never reaches the Connect codec.
			// The body is Go's plain text, not a `{"code": ...}` envelope.
			//
			// That matters to callers: connect-es surfaces this as a transport
			// error rather than a `ConnectError` with a numeric code, so a
			// client switching on `ConnectError.code` will not see
			// `unimplemented` here. Asserting the plain text keeps the fact
			// visible instead of implying an envelope that does not exist.
			const response = await rpc.post(procedure("NoSuchProcedure"), { data: {} });
			await expectStatus(response, 404);
			expect(await response.text()).toContain("404 page not found");
		},
	);

	test(
		"an unknown service path 404s",
		{ tag: "@contract" },
		async ({ rpc }) => {
			// The retired `services.backend.v1` name and any other service a
			// consolidation might try to co-mount here. `mainMux` resolves one
			// service prefix and nothing else.
			await expectStatus(
				await rpc.post("/services.sovereign.v2.KnowledgeSovereignService/GetLens", {
					data: {},
				}),
				404,
			);
			await expectStatus(
				await rpc.post("/alt.knowledge_home.v1.KnowledgeHomeService/GetKnowledgeHome", {
					data: {},
				}),
				404,
			);
		},
	);

	test(
		"a malformed JSON body is a client error, not a 500",
		{ tag: "@contract" },
		async ({ rpc }) => {
			// A body the codec cannot decode must be attributed to the caller.
			// A 500 here would page an operator for someone else's typo, and
			// would hide a genuine decoder panic behind the same status.
			const response = await rpc.post(procedure("GetKnowledgeHomeItems"), {
				headers: { "Content-Type": "application/json" },
				data: "{ this is not json",
			});
			expect(response.status()).toBeGreaterThanOrEqual(400);
			expect(response.status()).toBeLessThan(500);
			// connect-go answers a decode failure with a proper envelope
			// (unlike the unknown-procedure case above, which never reaches
			// the codec), so the SPA's connect-es client can classify it.
			const body = await expectJson(response, connectErrorSchema);
			expect(body.code).toBe("invalid_argument");
		},
	);
});

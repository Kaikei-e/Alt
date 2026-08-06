import { test, expect } from "../src/fixtures.js";
import { callUnary } from "../../_shared/connect.js";
import { expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { expectConnectionRefused } from "../../_shared/net.js";
import { env } from "../src/env.js";
import { latestArticleTimestampSchema } from "../src/schemas.js";

/**
 * Topology — the ports of `01-retired-connect-namespace-absent.hurl`,
 * `02-retired-internal-rest-absent.hurl`, the bottom half of
 * `03-ops-listener.hurl`, and the three `assert_transport_refused` loops that
 * lived in the Hurl `run.sh` because Hurl could not express them.
 *
 * The claim this file holds is that **alt-db's owner has exactly one door**.
 * `04`/`datahub-service.spec.ts` say the surface answers; on its own that is
 * satisfied by a service that answers everything to everyone. These negatives
 * are the other half, and the pair is the contract.
 *
 * Every assertion here is **404** or **connection refused**, never 401/403. A
 * 401 would mean the routes are still registered and only a middleware stands
 * between a caller and them; 404 is the only status that says "this surface is
 * not here". `cmd/datahub`'s router is explicit about it — anything outside
 * `/services.datahub.v1.` and `/health` goes to `http.NotFound`, and the
 * prefix is the full namespace rather than the `/services.` root *precisely*
 * so that `services.backend.v1` fails it too.
 */

const RETIRED_BACKEND_SVC = "/services.backend.v1.BackendInternalService";
const LEGACY_DATAHUB_SVC = "/alt.datahub.v1.DataHubService";
const SVC = "/services.datahub.v1.DataHubService";

/**
 * `services.backend.v1.BackendInternalService` — the name this surface
 * inherited from when the code lived inside alt-backend, retired by ADR-000954
 * Wave 2-C. One procedure per shape of caller, matching the Hurl file.
 */
const RETIRED_BACKEND_PROCEDURES = [
	{ procedure: "GetLatestArticleTimestamp", request: {} },
	{ procedure: "ListArticlesWithTags", request: { limit: 10 } },
	{
		procedure: "ListArticlesWithTagsForward",
		request: { incrementalMark: "2020-01-01T00:00:00Z", limit: 10 },
	},
	{ procedure: "ListDeletedArticles", request: { limit: 10 } },
	// pre-processor's write path — the one worth naming separately. A stale
	// caller on the old namespace must fail loudly rather than write through a
	// door nobody is auditing.
	{ procedure: "CheckArticleExists", request: { url: "https://stub.invalid/x", feedId: "00000000-0000-0000-0000-000000000001" } },
	{ procedure: "ListRecapArticles", request: {} },
] as const;

/** The `/v1/internal/*` REST pair ADR-000954 D6 folded into DataHubService. */
const RETIRED_INTERNAL_REST = [
	"/v1/internal/articles/recent",
	// Kept query-shaped on one entry so the request looks like the one
	// rag-orchestrator used to send. The router does not read query strings, so
	// this proves nothing the bare path does not — it is here so a reader can
	// see the retired call verbatim.
	"/v1/internal/articles/recent?within_hours=24&limit=10",
	"/v1/internal/system-user",
	// The whole prefix, not only the two routes that lived under it. A router
	// that still recognised /v1/internal and merely had no handlers registered
	// would be one DI line away from serving it again.
	"/v1/internal",
] as const;

test.describe("the retired Connect namespace answers nowhere", () => {
	for (const { procedure, request } of RETIRED_BACKEND_PROCEDURES) {
		test(`${RETIRED_BACKEND_SVC}/${procedure} → 404`, { tag: "@authz" }, async ({ dataHub }) => {
			// 404 rather than "not 200": a 501 or a Connect `unimplemented`
			// envelope would mean the mux still knows the service and something
			// else declined, which is a different and much less finished state.
			//
			// The caller is the allowed `pre-processor` leaf — the same identity
			// datahub-service.spec.ts uses. Probing with a peer that would be
			// refused at the handshake would make this pass for the wrong reason.
			const response = await callUnary(dataHub, `${RETIRED_BACKEND_SVC}/${procedure}`, request);
			await expectStatus(response, 404);
		});
	}

	test("the retired namespace 404s from the router, not from a Connect handler", { tag: "@authz" }, async ({
		dataHub,
	}) => {
		// Go's `http.NotFound` writes plain text. If this body were ever a
		// Connect error envelope it would mean `cmd/datahub`'s prefix router
		// passed the path through to a mux that recognised the service — a
		// compatibility shim re-added, which is exactly what Wave 2-C removed.
		const response = await callUnary(dataHub, `${RETIRED_BACKEND_SVC}/CreateArticle`, {});
		await expectStatus(response, 404);
		expect(await response.text()).toContain("404 page not found");
	});
});

test.describe("the ADR-000955 transitional alias", () => {
	/**
	 * `alt.datahub.v1.DataHubService` is **not** fenced yet, deliberately.
	 *
	 * ADR-000955 moved every proto package under the `services.` root, and
	 * consumers deployed before the rename still speak the old name — the pact
	 * broker's deployed-version selector keeps their old pacts in the provider
	 * verification matrix until each renamed consumer is recorded as deployed
	 * (alt-deploy run 30769818431 deadlocked on exactly this). `cmd/datahub`
	 * therefore ships `datahubapi.LegacyNamespaceAlias`, and these tests assert
	 * the alias is alive: a silent alias regression mid-transition would strand
	 * every deployed consumer.
	 *
	 * When the alias is removed, these two tests move into the describe above
	 * and become the same 404 fence.
	 */
	test("the legacy name answers like the current one", { tag: "@contract" }, async ({ dataHub }) => {
		// GetLatestArticleTimestamp is the probe because
		// datahub-service.spec.ts pins it to 200 on the canonical name, so a
		// failure here isolates the *alias* rather than the procedure.
		//
		// Strengthened past the Hurl original, which asserted only `HTTP 200`:
		// the alias is a path rewrite onto the same mux
		// (dataplane/connect/datahubapi/legacy_alias.go), so the body must be
		// the same envelope, not merely a success. A shim that answered 200
		// with something else would satisfy the old assertion.
		await expectJsonStatus(
			await callUnary(dataHub, `${LEGACY_DATAHUB_SVC}/GetLatestArticleTimestamp`, {}),
			200,
			latestArticleTimestampSchema,
		);
	});

	test("the alias rewrites a prefix, it does not blanket-accept", { tag: "@authz" }, async ({ dataHub }) => {
		// New coverage. `LegacyNamespaceAlias` cuts a fixed prefix and delegates;
		// an unknown procedure under the legacy name must therefore reach the
		// same 404 the current name gives it. If this ever answered, the alias
		// would have become a catch-all — a second door with no procedure list
		// behind it, which is the shape of the pre-split listener the whole
		// three-binary split existed to remove.
		await expectStatus(
			await callUnary(dataHub, `${LEGACY_DATAHUB_SVC}/NoSuchProcedureExists`, {}),
			404,
		);
	});

	test("the alias does not extend to sibling packages under alt.*", { tag: "@authz" }, async ({ dataHub }) => {
		// `LegacyNamespacePrefix` is `/alt.datahub.v1.DataHubService/` — the
		// whole service path, not `/alt.`. A user-facing service reachable here
		// would mean the data plane's socket had started answering for the
		// browser API again.
		await expectStatus(
			await callUnary(dataHub, "/alt.feeds.v2.FeedService/GetAllFeeds", {}),
			404,
		);
	});
});

test.describe("the retired /v1/internal REST pair answers nowhere", () => {
	for (const path of RETIRED_INTERNAL_REST) {
		test(`GET ${path} → 404`, { tag: "@authz" }, async ({ dataHub }) => {
			// These two handlers took no tenant argument and carried no auth
			// middleware; the listener was their entire access control. They were
			// not withdrawn but *converted* — ADR-000954 D6 folded them into
			// GetSystemUser and ListRecentArticles, which datahub-service.spec.ts
			// asserts answer. This file is the other half of that claim: the
			// capability moved rather than being duplicated.
			await expectStatus(await dataHub.get(path), 404);
		});
	}
});

test.describe("the operator listener carries nothing but /health and /metrics", () => {
	/**
	 * The plaintext-monitoring bargain only holds if that surface stays empty
	 * of everything else. `internal/bootstrap.NewOpsHandler` builds an explicit
	 * mux with two routes and deliberately never uses `http.DefaultServeMux` —
	 * which is where `net/http/pprof` registers itself via `init()`, so reusing
	 * it would publish heap and goroutine dumps from an unauthenticated port.
	 */
	const OPS_MUST_NOT_SERVE = [
		"GetLatestArticleTimestamp",
		"CreateArticle",
		// ListRecentArticles and GetSystemUser were REST routes under
		// /v1/internal until D6 folded them in. Probed by their *current* names:
		// a 404 for a path this listener never served proves less than a 404 for
		// the path the capability actually lives at today.
		"ListRecentArticles",
		"GetSystemUser",
	] as const;

	for (const procedure of OPS_MUST_NOT_SERVE) {
		test(`ops :9110 — ${procedure} → 404`, { tag: "@authz" }, async ({ ops }) => {
			// The data plane must not be reachable without a certificate by
			// walking in through the monitoring door.
			await expectStatus(await callUnary(ops, `${SVC}/${procedure}`, {}), 404);
		});
	}

	test("ops :9110 does not serve the public health route", { tag: "@authz" }, async ({ ops }) => {
		// `/v1/health` is alt-backend's browser-facing route. Its presence here
		// would mean the ops handler had been swapped for an Echo router.
		await expectStatus(await ops.get("/v1/health"), 404);
	});

	test("ops :9110 does not serve the retired internal REST pair", { tag: "@authz" }, async ({ ops }) => {
		// New coverage, and the mirror of the mTLS assertions above: the two
		// retired routes must be absent from *both* listeners, not merely from
		// the one anybody thought to check.
		await expectStatus(await ops.get("/v1/internal/system-user"), 404);
		await expectStatus(await ops.get("/v1/internal/articles/recent"), 404);
	});

	test("ops :9110 does not expose pprof", { tag: "@authz" }, async ({ ops }) => {
		// New coverage, grounded in NewOpsHandler's own comment about
		// http.DefaultServeMux. An unauthenticated goroutine dump is the
		// concrete thing that comment is defending against, so it is worth an
		// assertion rather than a comment alone.
		await expectStatus(await ops.get("/debug/pprof/"), 404);
		await expectStatus(await ops.get("/debug/pprof/goroutine"), 404);
	});
});

test.describe("mTLS only means there is no second door", () => {
	/**
	 * These are the ports the monolith listened on. `cmd/datahub` opens exactly
	 * two sockets — the mTLS `:9443` and the ops `:9110` — and
	 * `di/datahub.NewDataHubComponents` builds no admin surface for a third to
	 * serve, so a plaintext client must get connection-refused rather than a
	 * service.
	 *
	 * This is the one thing Hurl could not express: an entry that fails to
	 * reach the server is a run failure there, full stop, so the polarity had
	 * to be inverted outside the framework — a probe file asserting `HTTP *`
	 * plus a shell wrapper demanding exit code 3 so a parse error could not
	 * masquerade as a refusal (e2e/hurl/_lib/assert-transport-refused.sh).
	 * `expectConnectionRefused` keeps that distinction — an unrecognised error
	 * is a failure, not a refusal — as a normal assertion.
	 */
	const ABSENT_LISTENERS = [
		{
			what: "alt-data-hub serves no browser-facing REST API; those handlers are not compiled into cmd/datahub",
			url: () => env.absentRestURL,
			port: ":9000",
		},
		{
			what: "alt-data-hub serves no user-facing Connect listener; the user services stayed on cmd/backend",
			url: () => env.absentConnectURL,
			port: ":9101",
		},
		{
			// The tempting move during the split was to give alt-data-hub an
			// operator listener too — "it already has a plaintext port for the
			// probe". It does not: the admin Connect services stayed on
			// alt-backend's loopback listener, and OPERATOR_LISTEN_ADDR is
			// absent from this service's compose environment on purpose.
			what: "alt-data-hub has no operator Connect listener; the admin services did not move here",
			url: () => env.absentOperatorURL,
			port: ":9102",
		},
	] as const;

	for (const { what, url, port } of ABSENT_LISTENERS) {
		test(`nothing is listening on ${port}`, { tag: "@authz" }, async ({ prober }) => {
			await expectConnectionRefused(prober, `${url()}/health`, what);
		});
	}
});

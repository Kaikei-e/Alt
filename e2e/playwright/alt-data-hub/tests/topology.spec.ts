import { test, expect } from "../src/fixtures.js";
import { callUnary } from "../../_shared/connect.js";
import { expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { expectConnectionRefused } from "../../_shared/net.js";
import { env } from "../src/env.js";
import { latestArticleTimestampSchema } from "../src/schemas.js";

/** The verified peer/procedure gate denies retired or unknown data-plane paths.
 * Monitoring routes keep their separate 404 topology checks below.
 */
const RETIRED_BACKEND_SVC = "/services.backend.v1.BackendInternalService";
const LEGACY_DATAHUB_SVC = "/alt.datahub.v1.DataHubService";
const SVC = "/services.datahub.v1.DataHubService";

const RETIRED_BACKEND_PROCEDURES = [
	{ procedure: "GetLatestArticleTimestamp", request: {} },
	{ procedure: "ListArticlesWithTags", request: { limit: 10 } },
	{ procedure: "ListArticlesWithTagsForward", request: { incrementalMark: "2020-01-01T00:00:00Z", limit: 10 } },
	{ procedure: "ListDeletedArticles", request: { limit: 10 } },
	{ procedure: "CheckArticleExists", request: { url: "https://stub.invalid/x", feedId: "00000000-0000-0000-0000-000000000001" } },
	{ procedure: "ListRecapArticles", request: {} },
] as const;
const RETIRED_INTERNAL_REST = [
	"/v1/internal/articles/recent",
	"/v1/internal/articles/recent?within_hours=24&limit=10",
	"/v1/internal/system-user",
	"/v1/internal",
] as const;

test.describe("the procedure gate denies the retired Connect namespace", () => {
	for (const { procedure, request } of RETIRED_BACKEND_PROCEDURES) {
		test(`${RETIRED_BACKEND_SVC}/${procedure} → 403`, { tag: "@authz" }, async ({ dataHub }) => {
			await expectStatus(await callUnary(dataHub, `${RETIRED_BACKEND_SVC}/${procedure}`, request), 403);
		});
	}
	test("the retired namespace is rejected before Connect dispatch", { tag: "@authz" }, async ({ dataHub }) => {
		const response = await callUnary(dataHub, `${RETIRED_BACKEND_SVC}/CreateArticle`, {});
		await expectStatus(response, 403);
		expect(await response.text()).toContain("forbidden: unknown procedure");
	});
});

test.describe("the ADR-000955 transitional alias", () => {
	// Deployed legacy consumers use the same exact procedure capabilities.
	test("the legacy name answers like the current one", { tag: "@contract" }, async ({ dataHub }) => {
		await expectJsonStatus(await callUnary(dataHub, `${LEGACY_DATAHUB_SVC}/GetLatestArticleTimestamp`, {}), 200, latestArticleTimestampSchema);
	});
	test("the alias does not grant an unknown procedure", { tag: "@authz" }, async ({ dataHub }) => {
		const response = await callUnary(dataHub, `${LEGACY_DATAHUB_SVC}/NoSuchProcedureExists`, {});
		await expectStatus(response, 403);
		expect(await response.text()).toContain("forbidden: peer not authorized for this procedure");
	});
	test("the alias does not extend to sibling packages under alt.*", { tag: "@authz" }, async ({ dataHub }) => {
		await expectStatus(await callUnary(dataHub, "/alt.feeds.v2.FeedService/GetAllFeeds", {}), 403);
	});
});

test.describe("the procedure gate denies retired internal REST", () => {
	for (const path of RETIRED_INTERNAL_REST) {
		test(`GET ${path} → 403`, { tag: "@authz" }, async ({ dataHub }) => {
			await expectStatus(await dataHub.get(path), 403);
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

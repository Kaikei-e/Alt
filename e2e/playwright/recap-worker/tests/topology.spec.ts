import { test, expect } from "../src/fixtures.js";
import { expectNoHeader, expectStatus } from "../../_shared/http.js";
import { expectConnectionRefused } from "../../_shared/net.js";
import { env } from "../src/env.js";

/**
 * Listener topology — entirely new coverage, and the part the Hurl suite could
 * not have written at all.
 *
 * Hurl treats an entry that fails to reach the server as a run failure, full
 * stop, so "this port must have nothing on it" was inexpressible; the split
 * suites had to invert the polarity outside the framework with a shell wrapper
 * that demanded exit code 3 (e2e/hurl/_lib/assert-transport-refused.sh). In
 * Playwright `APIRequestContext` rejects on a transport error and resolves on
 * any HTTP status, so the fact is a normal assertion — see `_shared/net.ts`.
 *
 * There are two claims here worth stating plainly:
 *
 *   1. recap-worker serves the plaintext listener and *only* the plaintext
 *      listener in this configuration.
 *   2. The thing answering on BASE_URL is recap-worker, not the four-aliased
 *      stub sharing its network.
 *
 * The second sounds paranoid until you look at the slice:
 * `recap-pipeline-stub` answers to `rw-stub-subworker`,
 * `rw-stub-news-creator`, `rw-stub-alt-backend` and `rw-stub-tag-generator`,
 * serves `GET /health` with `{"status":"ok"}`, and has a catch-all that
 * answers **200 `{"status":"stub-noop"}` to every unmatched path**
 * (e2e/stubs/recap-pipeline-stub/main.py:455-465). A BASE_URL pointed at it
 * would make a large fraction of a naively-written suite pass.
 */

test.describe("listener topology", () => {
	test("the mutual-TLS listener is not bound in this slice @authz", async ({ api }) => {
		// `main.rs:188-219` spawns a second `axum_server::bind_rustls` listener
		// on `MTLS_PORT` (default 9443) serving `router.clone()` — the *entire*
		// API, every admin and dashboard route included — but only when
		// `tls::load_server_tls_config()` returns `Some`, which happens only
		// under `MTLS_ENFORCE=true` (tls.rs:54-57). The staging slice sets
		// `MTLS_ENFORCE=false`, so the correct state is "nothing listening".
		//
		// This is CLAUDE.md rule 9 checked from the outside: "disabled" has to
		// be an explicit config value with an observable consequence, not an
		// inference. A regression in `enforced()` that made the flag
		// default-on would republish the whole surface on a second port with
		// whatever certificate material happened to be around — and every
		// existing assertion in this suite, all aimed at :9005, would still
		// pass.
		//
		// `expectConnectionRefused` rather than `expectTlsHandshakeRejected`
		// on purpose: the two mean opposite things. A rejected handshake would
		// say "the mTLS listener is up and does not admit me", which is the
		// *enabled* configuration and is not what this slice runs.
		await expectConnectionRefused(
			api,
			`${env.mtlsURL}/health/live`,
			"recap-worker binds its rustls listener only under MTLS_ENFORCE=true; " +
				"the staging slice sets it false, so nothing may answer on MTLS_PORT",
		);
	});

	test("GET /health is a 404 — this is recap-worker, not the stub @contract", async ({ api }) => {
		// `api::router` registers `/health/live` and `/health/ready`, and no
		// bare `/health` (api.rs:24-25). The recap-pipeline-stub, which shares
		// this network under four aliases, registers exactly `/health` and
		// answers `{"status":"ok"}` — and its catch-all answers 200 to
		// everything else besides.
		//
		// So this single 404 is the control for the whole suite: it is the one
		// request that recap-worker and the stub answer differently in a way no
		// other assertion here would notice. If BASE_URL ever drifts onto a
		// `rw-stub-*` alias, this fails first and names why.
		await expectStatus(await api.get("/health"), 404);
	});

	test("an unregistered path answers 404 while a registered one does not @contract", async ({
		api,
	}) => {
		// The control that gives every 404 in this suite its meaning. axum's
		// fallback answers 404 for an unmatched route; `/health/live` is
		// matched. If both answered the same status, none of the negatives here
		// or in tests/validation.spec.ts would prove anything.
		await expectStatus(await api.get("/definitely-not-a-recap-worker-route"), 404);
		const mounted = await api.get("/health/live");
		expect(mounted.status(), "the control route must not 404").not.toBe(404);
	});

	test("responses leak no server-identity headers @contract", async ({ api }) => {
		// axum adds neither `Server` nor `X-Powered-By`, and `api::router`
		// installs no tower layer that would (api.rs:17-70). Asserting the
		// absence is the cheap regression fence for the day someone adds a
		// `ServiceBuilder` with a default set of response headers: version
		// banners are free reconnaissance, and a service that starts announcing
		// its framework version does so silently.
		const response = await api.get("/health/live");
		expectNoHeader(response, "Server");
		expectNoHeader(response, "X-Powered-By");
	});

	test("the whole surface is unauthenticated on this listener @authz", async ({ api }) => {
		// Not a gap being papered over — a fact this suite depends on and that
		// deserves to be written down. `api::router` has no auth layer, and the
		// slice runs `PEER_IDENTITY_TRUSTED=off`, so `/admin/jobs/retry` and
		// every `/v1/dashboard/*` route are reachable by anyone who can reach
		// the port. Reachability is the entire access control, which is exactly
		// why the test above (nothing bound on the mTLS port) is the one that
		// matters.
		//
		// A peer-identity header is inert here — no handler reads one — and
		// asserting that pins it: the day an identity layer is added, this test
		// is where the decision about which routes it covers gets made instead
		// of being assumed.
		const response = await api.get("/v1/dashboard/job-stats", {
			headers: { "X-Alt-Peer-Identity": "someone-else" },
		});
		await expectStatus(response, 200);
	});
});

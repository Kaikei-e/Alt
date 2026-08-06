import { expect, test } from "../src/fixtures.js";
import {
	expectJsonStatus,
	expectNoHeader,
	expectStatus,
	expectStatusIn,
} from "../../_shared/http.js";
import { expectConnectionRefused } from "../../_shared/net.js";
import { env, Procedure, SharedCorpus } from "../src/env.js";
import { nonEmptySearchResponseSchema } from "../src/schemas.js";

/**
 * Listener topology and the access-control posture that follows from it — new
 * coverage, and the part of this service the Hurl suite could not express at
 * all.
 *
 * search-indexer binds up to three listeners, and which ones is a
 * configuration decision made in `bootstrap/app.go`:
 *
 *   :9300  REST     — always
 *   :9301  Connect  — always
 *   :9443  REST + Connect behind mutual TLS — **only** when
 *          `os.Getenv("MTLS_LISTEN") == "true"`
 *
 * The consequence is stated plainly in `bootstrap/servers.go`: *"/v1/search is
 * gated at the transport layer (mTLS peer-identity on the :9443 listener). The
 * plaintext :9300 path here serves only rate-limited handlers; auth has been
 * removed pending retirement of the listener itself."* On the plaintext ports,
 * **network reachability is the entire access control** for an endpoint that
 * can read any tenant's articles. That is a deliberate migration-window
 * posture, not an oversight — but nothing was asserting it, so nothing would
 * have noticed it changing in either direction.
 *
 * Every negative below asserts **404**, never 401 or 403. A 401 would mean the
 * route is registered on the wrong mux and only a middleware stands between a
 * caller and it; 404 is the only status that says "this surface is not here".
 */

test.describe("the plaintext listeners answer only their own routes", () => {
	test("the positive control", { tag: "@smoke" }, async ({ rest, bare }) => {
		// Without this every 404 below could be passing because the listener is
		// down, which is the classic way a negative-assertion file reports green
		// on a completely broken deployment.
		await expectStatus(await rest.get("/health"), 200);
		await expectStatus(await rest.get("/v1/search?q=rust&limit=1"), 200);
		await expectStatus(await bare.get(`${env.connectURL}/health`), 200);
	});

	test("REST :9300 does not serve Connect procedures", { tag: "@contract" }, async ({
		rest,
	}) => {
		// The :9443 mux deliberately merges the two surfaces —
		// `newMTLSMuxHandler` registers `/v1/search`, `/health`, the
		// `/services.search.v2.SearchService/` prefix *and* a catch-all `/` that
		// forwards to Connect. The plaintext mux must not, or the peer-identity
		// gate that mTLS mux exists to apply would be bypassable by switching
		// port.
		await expectStatus(
			await rest.post(`/${Procedure.searchArticles}`, {
				headers: { "Content-Type": "application/json" },
				data: {},
			}),
			404,
		);
	});

	test("Connect :9301 does not serve the REST search route", { tag: "@contract" }, async ({
		bare,
	}) => {
		// The other direction. `CreateConnectServer`'s mux registers `/health`
		// and the service prefix and nothing else, so `/v1/search` here is an
		// unmatched route — not a second, differently-configured copy of the
		// search handler.
		await expectStatus(await bare.get(`${env.connectURL}/v1/search?q=rust`), 404);
	});

	for (const [listener, url] of [
		["REST :9300", () => env.baseURL],
		["Connect :9301", () => env.connectURL],
	] as const) {
		test(`${listener} has no catch-all root handler`, { tag: "@contract" }, async ({
			bare,
		}) => {
			// `newMTLSMuxHandler` ends with `mux.Handle("/", connect)`. Neither
			// plaintext mux has that line, and it matters: a catch-all on a
			// plaintext port would make every future Connect service reachable
			// there the moment it is registered, without anyone editing this
			// file or noticing.
			await expectStatus(await bare.get(`${url()}/`), 404);
		});
	}

	test("REST :9300 exposes no /metrics", { tag: "@authz" }, async ({ rest }) => {
		// search-indexer publishes telemetry over OTLP
		// (`OTEL_EXPORTER_OTLP_ENDPOINT`), not by scraping — `newHTTPServer`
		// registers exactly two routes. Asserting the absence is what stops a
		// promhttp handler being added to the *unauthenticated* listener as a
		// convenience: on this port that would publish per-query cardinality to
		// anyone who can reach the container.
		await expectStatus(await rest.get("/metrics"), 404);
	});

	test("no listener advertises a Server banner", { tag: "@contract" }, async ({ rest }) => {
		// Go's net/http sets no `Server` header and nothing in
		// `bootstrap/servers.go` adds one. Cheap to assert, and the thing it
		// fences is a reverse proxy or middleware being introduced in front of
		// the service without anyone deciding what it should leak.
		const response = await rest.get("/health");
		expectNoHeader(response, "Server");
		expectNoHeader(response, "X-Powered-By");
	});
});

test.describe("the mutual-TLS listener is opt-in", () => {
	test("nothing answers on :9443 when MTLS_LISTEN is false", { tag: "@authz" }, async ({
		bare,
	}) => {
		// compose.staging.yaml sets `MTLS_LISTEN=false`, and `bootstrap/app.go`
		// binds the mTLS mux only on the exact string `"true"`. So the correct
		// observable state is *nothing bound* — which is the CLAUDE.md rule 9
		// shape: "disabled" is an explicit config value, and its consequence is
		// asserted rather than inferred.
		//
		// This is the assertion Hurl could not make at all: it treats a
		// connection it cannot establish as a run failure, so the split-topology
		// Hurl suites had to invert the polarity outside the framework with a
		// shell wrapper demanding exit code 3.
		//
		// If someone flips MTLS_LISTEN on in the staging slice, this fails — and
		// it should, because the suite would then need `suite_pki` and a client
		// certificate, and every "plaintext is the only surface" claim above
		// would need revisiting.
		await expectConnectionRefused(
			bare,
			`${env.mtlsAbsentURL}/health`,
			"search-indexer binds its mutual-TLS mux only when MTLS_LISTEN=true; the " +
				"staging slice sets it false, so :9443 must not be listening",
		);
	});
});

test.describe("access-control posture on the plaintext ports", () => {
	test("searching requires no credential whatsoever", { tag: "@authz" }, async ({ bare }) => {
		// Stated as a test rather than left in a README, because it is the whole
		// reason the mTLS listener exists and the whole reason this suite needs
		// no auth fixture. `bare` sends no cookie, no bearer token, no client
		// certificate and no peer-identity header.
		//
		// When `/v1/search` does start requiring a credential on this port —
		// which is what "pending retirement of the listener itself" in
		// bootstrap/servers.go anticipates — this test fails. That is the
		// intended signal, not a bug in the test.
		//
		// The posture claim is that the credential-free path is *served*, not
		// merely un-rejected, so the hit is asserted rather than the status
		// alone. `q=rust` matches exactly the two shared-corpus documents
		// globalSetup indexed before any worker started, and `limit=1` caps the
		// page at one — so `hits: []` under a 200, which is what a search path
		// that stopped reaching Meilisearch returns forever, fails here.
		const body = await expectJsonStatus(
			await bare.get(
				`${env.baseURL}/v1/search?q=${SharedCorpus.rustQuery}&limit=1`,
			),
			200,
			nonEmptySearchResponseSchema,
		);
		expect(body.hits).toHaveLength(1);
	});

	test("a spoofed peer-identity header changes nothing", { tag: "@authz" }, async ({
		rest,
		bare,
		corpus,
	}) => {
		// `X-Alt-Peer-Identity` is what `PeerIdentityMiddleware` *sets* from a
		// verified client certificate CN, after stripping whatever the client
		// sent. That middleware is wired only into `newMTLSMuxHandler`, so on
		// :9300 the header is untrusted input that nothing reads.
		//
		// This pins **current** behaviour. If a future change starts trusting
		// the header without an mTLS listener in front to overwrite it, this
		// test breaks — which is exactly the alarm worth having, because the
		// header is trivially forgeable over plaintext.
		//
		// Both sides are parsed as non-empty and the honest side is pinned to
		// the whole seeded corpus first: an equality between two empty lists is
		// trivially true, so a search path that had stopped reaching
		// Meilisearch would otherwise satisfy "the header changed nothing" while
		// changing everything.
		const path = `/v1/search?q=${corpus.nonce}&user_id=${corpus.userId}&limit=10`;
		const honest = await expectJsonStatus(
			await rest.get(path),
			200,
			nonEmptySearchResponseSchema,
		);
		expect(honest.hits).toHaveLength(corpus.docs.length);

		const spoofed = await expectJsonStatus(
			await bare.get(`${env.baseURL}${path}`, {
				headers: { "X-Alt-Peer-Identity": "alt-backend" },
			}),
			200,
			nonEmptySearchResponseSchema,
		);
		expect(spoofed.hits.map((hit) => hit.id).sort()).toEqual(
			honest.hits.map((hit) => hit.id).sort(),
		);
	});

	test("Meilisearch itself is not open to the network", { tag: "@authz" }, async ({ bare }) => {
		// The corpus lives in a Meilisearch on the same internal network, and
		// this suite writes to it with the master key from
		// `e2e/fixtures/staging-secrets/`. If that key were not actually being
		// enforced, every seed and every tenant-isolation assertion above would
		// still pass while the index was in fact world-writable.
		//
		//   401  no `Authorization` header at all — what `bare` sends
		//   403  a header present but not authorised for this route
		//
		// Both mean the key is enforced. A **200** would mean
		// `MEILI_MASTER_KEY` never took effect, which is the finding.
		const response = await bare.get(`${env.meiliURL}/indexes`);
		await expectStatusIn(response, [401, 403]);
	});
});

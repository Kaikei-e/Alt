import { test, expect } from "../src/fixtures.js";
import { expectTlsHandshakeRejected } from "../../_shared/net.js";
import { env } from "../src/env.js";
import { assertAnswered, assertRefusedAtHandshake, probeMTLS } from "../src/mtls-probe.js";

/**
 * The mutual-TLS boundary — the port of the two `assert_transport_refused`
 * calls the Hurl `run.sh` had to make in shell, plus the positive control they
 * never had.
 *
 * There is no JWT on this listener. `tlsutil.LoadServerConfig` is built with
 * `tls.RequireAndVerifyClientCert` as a **constant**, not a setting — the
 * listener it replaced read `MTLS_CLIENT_AUTH` and fell back to
 * `tls.NoClientCert` whenever the variable was unset, which made "mTLS
 * boundary" a claim the deployment might not honour — and
 * `tlsutil.WithAllowedPeers` adds a `VerifyConnection` callback on top. So
 * both negatives below are *handshake* failures: there is no status code to
 * assert on, because an unauthenticated caller never reaches a route and
 * therefore no route-level mistake could ever expose one.
 *
 * The three tests read together. On its own the anonymous rejection is
 * satisfied by a port that is simply down, and the positive control on its own
 * is satisfied by a listener that admits everybody.
 */

test.describe("who the data plane admits", () => {
	test("an anonymous TLS client is rejected in the handshake", { tag: "@authz" }, async ({ anonTLS }) => {
		// `anonTLS` sets `ignoreHTTPSErrors`, and that is load-bearing rather
		// than convenience: without it the handshake would fail with
		// "self-signed certificate in chain" — this client's complaint about a
		// throwaway CA it was never told about — which would satisfy the
		// assertion while proving nothing at all about client authentication.
		// With it, the only remaining reason the handshake can fail is the
		// server demanding a certificate, which is the fact under test.
		await expectTlsHandshakeRejected(
			anonTLS,
			`${env.dataHubURL}/health`,
			"alt-data-hub's data plane requires a client certificate " +
				"(tls.RequireAndVerifyClientCert, hard-coded in cmd/datahub/main.go)",
		);
	});

	// The title does not interpolate DENIED_PEER: a test title that changes
	// with the environment breaks `--grep` and makes two runs' reports
	// un-diffable. The peer name is in the failure message instead.
	test("a valid chain with the wrong identity is rejected in the handshake", { tag: "@authz" }, async () => {
		// Service B must not be able to impersonate service A even holding a
		// leaf from the same internal CA — which is the whole reason
		// DATAHUB_ALLOWED_PEERS exists on top of `RequireAndVerify`. Both
		// certificates here chain to the same root; only the subject differs.
		//
		// This uses `src/mtls-probe.ts` rather than a Playwright request
		// context because sending a client certificate makes Playwright proxy
		// the connection through its own TLS terminator, so the error the
		// caller sees is that proxy's and says nothing about the peer's alert.
		// The probe also discriminates the case that matters most: a **403**
		// would mean the handshake succeeded and only
		// `middleware.RequirePeerIdentity` caught it — the per-request
		// re-check doing a job `WithAllowedPeers` is supposed to have already
		// done, which is a real regression and not a pass.
		const result = await probeMTLS({
			host: env.dataHubHost,
			port: env.dataHubPort,
			path: "/health",
			caPath: env.caPath,
			certPath: env.deniedCertPath,
			keyPath: env.deniedKeyPath,
		});

		assertRefusedAtHandshake(
			result,
			`${env.deniedPeer} holds a leaf from this run's CA but is not in ` +
				`DATAHUB_ALLOWED_PEERS, so tlsutil.WithAllowedPeers' VerifyConnection ` +
				`callback must reject it before any HTTP is spoken`,
		);
	});

	test("an allowed peer completes the handshake against this run's CA", { tag: "@smoke" }, async () => {
		// The positive control, and the one assertion in the suite that verifies
		// the *server's* certificate. Every Playwright context here runs with
		// `ignoreHTTPSErrors: true` because there is no way to hand an
		// APIRequestContext a custom root; this probe supplies `ca` explicitly
		// and leaves verification on, so it proves the leaf alt-data-hub serves
		// chains to the CA `suite_pki` minted and carries `alt-data-hub` as its
		// name.
		//
		// Without it the two negatives above would be satisfied by a port that
		// nothing is listening on — `assertRefusedAtHandshake` guards against
		// ECONNREFUSED specifically, but a listener that rejected *everyone*
		// would still pass them both.
		const result = await probeMTLS({
			host: env.dataHubHost,
			port: env.dataHubPort,
			path: "/health",
			caPath: env.caPath,
			certPath: env.allowedCertPath,
			keyPath: env.allowedKeyPath,
		});

		assertAnswered(
			result,
			200,
			`${env.allowedPeer} is in DATAHUB_ALLOWED_PEERS, so the handshake must ` +
				`complete and /health must answer`,
		);
	});

	test("the mTLS listener answers /health only after the certificate is accepted", { tag: "@authz" }, async ({
		dataHub,
		anonTLS,
	}) => {
		// The same route, the same bytes on the wire, two identities. Stating it
		// as one test is what makes the pair's meaning explicit: the credential
		// is the *connection*, not anything in the request, so there is no
		// header a caller can add or omit to change the outcome.
		const authed = await dataHub.get("/health");
		expect(authed.status()).toBe(200);

		await expectTlsHandshakeRejected(
			anonTLS,
			`${env.dataHubURL}/health`,
			"the same route that answers an allowed peer must not answer an anonymous one",
		);
	});
});

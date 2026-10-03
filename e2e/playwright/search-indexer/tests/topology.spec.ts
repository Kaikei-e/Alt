import { expect, test } from "../src/fixtures.js";
import {
	expectJsonStatus,
	expectNoHeader,
	expectStatus,
	expectStatusIn,
} from "../../_shared/http.js";
import { expectConnectionRefused, expectTlsHandshakeRejected } from "../../_shared/net.js";
import { clientCertificates } from "../../_shared/client-auth.js";
import { env, Procedure, SharedCorpus } from "../src/env.js";
import { nonEmptySearchResponseSchema } from "../src/schemas.js";

/**
 * Asserts that the request fails specifically due to the TLS handshake rejecting
 * an unauthorized client certificate with SSL alert 42 (bad certificate).
 * Generic connection resets, timeouts, or refused connections are never accepted.
 */
function isTlsBadCertificate(errorMsg: string): boolean {
	const hasBadCertAlert =
		/ssl\/tls alert bad certificate/i.test(errorMsg) ||
		/SSL alert number 42/i.test(errorMsg) ||
		/alert bad certificate/i.test(errorMsg);
	if (!hasBadCertAlert) {
		return false;
	}
	const isGeneric =
		/ECONNRESET/i.test(errorMsg) ||
		/ECONNREFUSED/i.test(errorMsg) ||
		/ETIMEDOUT/i.test(errorMsg) ||
		/socket hang up/i.test(errorMsg);
	return !isGeneric;
}

async function expectTlsBadCertificate(
	action: Promise<unknown>,
	what: string,
): Promise<void> {
	try {
		const res = await action;
		const status = (res && typeof res === "object" && "status" in res && typeof res.status === "function")
			? res.status()
			: JSON.stringify(res);
		throw new Error(
			`${what}\n  expected: TLS handshake rejected with bad certificate alert (alert 42)\n` +
				`  actual:   server answered with HTTP ${status}`,
		);
	} catch (error) {
		if (error instanceof Error && error.message.startsWith(what)) {
			throw error;
		}
		const msg = error instanceof Error ? error.message : String(error);
		if (!isTlsBadCertificate(msg)) {
			throw new Error(
				`${what}\n  expected: TLS handshake rejected with bad certificate alert (alert 42)\n` +
					`  actual error: ${msg}`,
			);
		}
	}
}

/** Plaintext :9300 serves health only; business REST/Connect routes require mTLS on :9443. */

test.describe("the plaintext listeners answer only their own routes", () => {
	test("the positive control", { tag: "@smoke" }, async ({ rest, bare }) => {
		// Without this every 404 below could be passing because the listener is
		// down, which is the classic way a negative-assertion file reports green
		// on a completely broken deployment.
		await expectStatus(await rest.get("/health"), 200);
		await expectStatus(await rest.get(`/v1/search?q=rust&user_id=${SharedCorpus.aliceUser}&limit=1`), 200);
		await expectStatus(await bare.get(`${env.plaintextURL}/health`), 200);
	});

	test("plaintext :9300 does not serve Connect procedures", { tag: "@contract" }, async ({
		bare,
	}) => {
		// The :9443 mux deliberately merges the two surfaces —
		// `newMTLSMuxHandler` registers `/v1/search`, `/health`, the
		// `/services.search.v2.SearchService/` prefix *and* a catch-all `/` that
		// forwards to Connect. The plaintext mux must not, or the peer-identity
		// gate that mTLS mux exists to apply would be bypassable by switching
		// port.
		await expectStatus(
			await bare.post(`${env.plaintextURL}/${Procedure.searchArticles}`, {
				headers: { "Content-Type": "application/json" },
				data: {},
			}),
			404,
		);
	});

	test("plaintext :9300 does not serve business search", { tag: "@contract" }, async ({
		bare,
	}) => {
		await expectStatus(
			await bare.get(`${env.plaintextURL}/v1/search?q=rust&user_id=${SharedCorpus.aliceUser}&limit=1`),
			404,
		);
	});

	test("legacy Connect :9301 port is refused", { tag: "@contract" }, async ({
		bare,
	}) => {
		await expectConnectionRefused(
			bare,
			`${env.retiredConnectURL}/health`,
			"legacy :9301 listener has been retired; search-indexer listens on :9300 and mTLS :9443",
		);
	});

	for (const [listener, url] of [
		["REST :9300", () => env.plaintextURL],
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

	test("REST :9300 exposes no /metrics", { tag: "@authz" }, async ({ bare }) => {
		// search-indexer publishes telemetry over OTLP
		// (`OTEL_EXPORTER_OTLP_ENDPOINT`), not by scraping — `newHTTPServer`
		// registers exactly two routes. Asserting the absence is what stops a
		// promhttp handler being added to the *unauthenticated* listener as a
		// convenience: on this port that would publish per-query cardinality to
		// anyone who can reach the container.
		await expectStatus(await bare.get(`${env.plaintextURL}/metrics`), 404);
	});

	test("no listener advertises a Server banner", { tag: "@contract" }, async ({ bare }) => {
		// Go's net/http sets no `Server` header and nothing in
		// `bootstrap/servers.go` adds one. Cheap to assert, and the thing it
		// fences is a reverse proxy or middleware being introduced in front of
		// the service without anyone deciding what it should leak.
		const response = await bare.get(`${env.plaintextURL}/health`);
		expectNoHeader(response, "Server");
		expectNoHeader(response, "X-Powered-By");
	});
});

test.describe("the mutual-TLS listener enforces peer identity", () => {
	test("TLS business endpoints reject uncredentialed client at handshake", { tag: "@authz" }, async ({
		bare,
	}) => {
		await expectTlsHandshakeRejected(
			bare,
			`${env.baseURL}/v1/search?q=rust&user_id=${SharedCorpus.aliceUser}&limit=1`,
			"search-indexer mTLS listener rejects uncredentialed TLS clients during handshake",
		);
		await expectTlsHandshakeRejected(
			bare,
			`${env.connectURL}/${Procedure.searchArticles}`,
			"search-indexer mTLS listener rejects uncredentialed TLS clients during handshake",
		);
	});

	test("TLS business endpoints reject unauthorized peer certificate at handshake", { tag: "@authz" }, async ({
		playwright,
	}) => {
		const denied = await playwright.request.newContext({
			clientCertificates: clientCertificates(env.baseURL, env.deniedCert, env.deniedKey),
		});
		try {
			await expectTlsBadCertificate(
				denied.get(`${env.baseURL}/v1/search?q=rust&user_id=${SharedCorpus.aliceUser}&limit=1`),
				"search-indexer mTLS listener rejects unauthorized peer certificate at handshake",
			);
			await expectTlsBadCertificate(
				denied.post(`${env.connectURL}/${Procedure.searchArticles}`, {
					headers: { "Content-Type": "application/json" },
					data: {},
				}),
				"search-indexer mTLS listener rejects unauthorized peer certificate at handshake",
			);
		} finally {
			await denied.dispose();
		}
	});

	test("TLS health rejects uncredentialed client at handshake", { tag: "@contract" }, async ({
		bare,
	}) => {
		await expectTlsHandshakeRejected(
			bare,
			`${env.baseURL}/health`,
			"search-indexer mTLS listener requires client certificate on :9443 even for /health",
		);
	});
});

test.describe("access-control posture on business endpoints", () => {
	test("searching requires an owner token", { tag: "@authz" }, async ({ rest }) => {
		// Compare unauthenticated rejection with a real nonempty authorized result.
		await expectStatus(
			await rest.get(
				`/v1/search?q=${SharedCorpus.rustQuery}&user_id=${SharedCorpus.aliceUser}&limit=1`,
				{ headers: { Authorization: "" } },
			),
			401,
		);
		const body = await expectJsonStatus(
			await rest.get(
				`/v1/search?q=${SharedCorpus.rustQuery}&user_id=${SharedCorpus.aliceUser}&limit=1`,
			),
			200,
			nonEmptySearchResponseSchema,
		);
		expect(body.hits).toHaveLength(1);
	});

	test("searching without user_id is rejected with 400", { tag: "@authz" }, async ({
		rest,
	}) => {
		const response = await rest.get(
			`/v1/search?q=${SharedCorpus.rustQuery}&limit=1`,
		);
		await expectStatus(response, 400);
	});

	test("a spoofed peer-identity header changes nothing", { tag: "@authz" }, async ({
		rest,
		playwright,
		corpus,
	}) => {
		// `X-Alt-Peer-Identity` is what `PeerIdentityMiddleware` *sets* from a
		// verified client certificate CN, after stripping whatever the client
		// sent.
		const path = `/v1/search?q=${corpus.nonce}&user_id=${corpus.userId}&limit=10`;
		const honest = await expectJsonStatus(
			await rest.get(path),
			200,
			nonEmptySearchResponseSchema,
		);
		expect(honest.hits).toHaveLength(corpus.docs.length);

		const denied = await playwright.request.newContext({
			baseURL: env.baseURL,
			clientCertificates: clientCertificates(env.baseURL, env.deniedCert, env.deniedKey),
			extraHTTPHeaders: { "X-Alt-Peer-Identity": "alt-backend" },
		});
		try {
			await expectTlsBadCertificate(
				denied.get(path),
				"denied peer certificate is rejected at handshake before HTTP headers are evaluated",
			);
		} finally {
			await denied.dispose();
		}
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

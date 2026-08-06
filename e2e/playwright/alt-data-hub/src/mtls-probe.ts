import { readFileSync } from "node:fs";
import { connect as tlsConnect } from "node:tls";

/**
 * A raw TLS probe that carries a client certificate.
 *
 * ### Why this is not `_shared/net.ts`
 *
 * `expectTlsHandshakeRejected` covers the anonymous case perfectly: an
 * `APIRequestContext` with no client certificate is plain undici, so a
 * rejected handshake surfaces as a normal Node TLS error and the shared
 * helper's pattern match is meaningful.
 *
 * It cannot cover the *identity* case. To send a client certificate Playwright
 * routes the request through an internal TLS-terminating proxy, and what
 * reaches the caller when the upstream rejects the handshake is that proxy's
 * error, not the peer's alert — so the shared helper would be matching on text
 * that says nothing about who alt-data-hub admits. That is exactly the failure
 * mode `_shared/net.ts` warns about in its own header ("an unrecognised error
 * is a failure, not a refusal"), so the honest move is a probe that speaks TLS
 * itself.
 *
 * ### Why it sends a request instead of stopping at the handshake
 *
 * Under TLS 1.3 the client sends its certificate *after* the server's
 * Finished, so `secureConnect` can fire on a connection the server is about to
 * tear down — a probe that resolved there would report "established" for a
 * peer that was in fact refused. Writing a minimal HTTP/1.1 request and
 * waiting for the first byte collapses that ambiguity, and it buys a third
 * outcome worth distinguishing:
 *
 *   - a transport error / silent close → the boundary refused the caller
 *   - **HTTP 403** → the handshake *succeeded* and only
 *     `middleware.RequirePeerIdentity` (alt-backend/app/middleware/
 *     require_peer_identity.go) caught it. That means
 *     `tlsutil.WithAllowedPeers`' VerifyConnection callback stopped running —
 *     a real regression that a plain "it failed somehow" assertion would call
 *     a pass.
 *   - HTTP 2xx → the boundary is open
 *
 * The CA is always supplied and server verification is always on, which is
 * the one thing the Playwright contexts in `fixtures.ts` give up
 * (`ignoreHTTPSErrors: true`). The positive probe in tests/mtls-boundary.spec.ts
 * is therefore also the suite's proof that alt-data-hub is serving the leaf
 * this run's CA issued.
 */

export type MTLSProbeResult =
	| {
			readonly kind: "answered";
			/** e.g. `HTTP/1.1 200 OK` */
			readonly statusLine: string;
			readonly status: number;
	  }
	| { readonly kind: "refused"; readonly error: string }
	/**
	 * The socket went quiet without answering and without erroring.
	 *
	 * A distinct kind, not a flavour of "refused": a rejected handshake is
	 * instant, so a timeout means the listener is hung or starved, not that it
	 * turned this identity away. Folding the two together would let a
	 * pool-exhausted alt-data-hub satisfy every authz negative in the suite.
	 */
	| { readonly kind: "timeout"; readonly afterMs: number };

export type MTLSProbeOptions = {
	readonly host: string;
	readonly port: number;
	/** Request target. `/health` is the cheapest route on the mTLS listener. */
	readonly path: string;
	/** Root the server leaf must chain to. Verification is never disabled. */
	readonly caPath: string;
	/** Omit both to probe as an anonymous TLS client. */
	readonly certPath?: string;
	readonly keyPath?: string;
	/** SNI / hostname to verify against. Defaults to `host`. */
	readonly servername?: string;
	/** A refused handshake is instant, so a slow one is news. */
	readonly timeoutMs?: number;
};

const DEFAULT_TIMEOUT_MS = 10_000;

export function probeMTLS(options: MTLSProbeOptions): Promise<MTLSProbeResult> {
	const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS;
	const servername = options.servername ?? options.host;

	return new Promise<MTLSProbeResult>((resolve) => {
		let settled = false;
		let received = "";

		const socket = tlsConnect({
			host: options.host,
			port: options.port,
			servername,
			ca: readFileSync(options.caPath),
			...(options.certPath === undefined
				? {}
				: { cert: readFileSync(options.certPath) }),
			...(options.keyPath === undefined ? {} : { key: readFileSync(options.keyPath) }),
			// http/1.1 only. Go's TLS config advertises h2 as well, and a probe
			// that negotiated it would have to speak HTTP/2 frames to read a
			// status line — which is a lot of machinery for a fact that is about
			// TLS, not about the application protocol.
			ALPNProtocols: ["http/1.1"],
		});

		const finish = (result: MTLSProbeResult): void => {
			if (settled) return;
			settled = true;
			socket.destroy();
			resolve(result);
		};

		socket.setTimeout(timeoutMs);

		socket.on("secureConnect", () => {
			socket.write(
				`GET ${options.path} HTTP/1.1\r\n` +
					`Host: ${options.host}:${options.port}\r\n` +
					`Connection: close\r\n\r\n`,
			);
		});

		socket.on("data", (chunk: Buffer) => {
			received += chunk.toString("utf8");
			const end = received.indexOf("\r\n");
			if (end < 0) return;
			const statusLine = received.slice(0, end);
			const match = /^HTTP\/[\d.]+ (\d{3})/.exec(statusLine);
			finish({
				kind: "answered",
				statusLine,
				status: match?.[1] === undefined ? 0 : Number.parseInt(match[1], 10),
			});
		});

		socket.on("error", (error: Error) => {
			finish({ kind: "refused", error: error.message });
		});

		socket.on("timeout", () => {
			finish({ kind: "timeout", afterMs: timeoutMs });
		});

		// A clean close before any byte arrived is still a refusal — Go closes
		// the connection after a failed VerifyConnection rather than answering —
		// but it is a *different* one from an error, so it is named separately.
		socket.on("close", () => {
			finish({ kind: "refused", error: "the peer closed the connection without answering" });
		});
	});
}

/**
 * Errors that mean **nothing is listening**, which must never be mistaken for
 * "the listener refused this identity".
 *
 * This is the same discipline `_shared/net.ts` applies in the other direction:
 * a probe that cannot reach the port proves nothing about who the port admits,
 * and accepting it here would let the mTLS listener disappear entirely while
 * every authz negative still reported green.
 */
const NOT_LISTENING_PATTERNS: readonly RegExp[] = [
	/ECONNREFUSED/i,
	/EHOSTUNREACH/i,
	/ENETUNREACH/i,
	/ENOTFOUND/i,
	/EAI_AGAIN/i,
];

/**
 * Errors that actually mean **the peer rejected this client's certificate**.
 *
 * An allowlist, not a denylist, and that distinction is the whole point.
 * Listing only the failures to *reject* leaves every unanticipated error
 * counting as a pass — including a server-leaf verification failure, which is
 * reachable here because `probeMTLS` always supplies the CA and leaves
 * verification on. An expired or wrong-SAN server certificate is a
 * misconfiguration on the *server* side; reading it as "the allowlist turned
 * me away" is the same silent-pass shape `_shared/net.ts` was written to
 * avoid, and this helper had inverted it.
 */
const HANDSHAKE_REJECTED_PATTERNS: readonly RegExp[] = [
	/EPROTO/i,
	/ECONNRESET/i,
	/SSL routines/i,
	/alert (handshake failure|bad certificate|certificate required|unknown ca|certificate unknown)/i,
	/socket hang up/i,
	// What `probeMTLS` reports when Go closes the connection after a failed
	// VerifyConnection rather than sending an alert — the common case here.
	/closed the connection without answering/i,
];

/**
 * Client-side verification failures. These are about the *server's* leaf, so
 * they can never evidence a decision about the client's identity.
 */
const SERVER_CERT_PATTERNS: readonly RegExp[] = [
	/unable to verify the first certificate/i,
	/self.signed certificate/i,
	/certificate has expired/i,
	/altnames/i,
	/Hostname\/IP does not match/i,
];

/** Asserts the peer refused this client at the transport, not at HTTP. */
export function assertRefusedAtHandshake(result: MTLSProbeResult, what: string): void {
	if (result.kind === "answered") {
		if (result.status === 403) {
			throw new Error(
				`${what}\n  expected: the TLS handshake is rejected\n` +
					`  actual:   the handshake SUCCEEDED and the request was refused at HTTP ` +
					`(${result.statusLine}).\n` +
					`  That is middleware.RequirePeerIdentity catching it, which means ` +
					`tlsutil.WithAllowedPeers' VerifyConnection callback is no longer ` +
					`running — the allowlist is now enforced one layer too late.`,
			);
		}
		throw new Error(
			`${what}\n  expected: the TLS handshake is rejected\n` +
				`  actual:   the server answered "${result.statusLine}" — this boundary is open`,
		);
	}
	if (result.kind === "timeout") {
		throw new Error(
			`${what}\n  the socket went quiet for ${result.afterMs}ms without answering and ` +
				`without erroring. A rejected handshake is instant, so this is a hung or ` +
				`starved listener — it says nothing about who the listener admits.`,
		);
	}
	for (const pattern of NOT_LISTENING_PATTERNS) {
		if (pattern.test(result.error)) {
			throw new Error(
				`${what}\n  nothing is listening on that address, so this proves nothing ` +
					`about who the listener admits.\n  error: ${result.error}`,
			);
		}
	}
	for (const pattern of SERVER_CERT_PATTERNS) {
		if (pattern.test(result.error)) {
			throw new Error(
				`${what}\n  the probe rejected the *server's* certificate, so it never got far ` +
					`enough to learn what the server thinks of this client.\n  error: ${result.error}`,
			);
		}
	}
	if (!HANDSHAKE_REJECTED_PATTERNS.some((pattern) => pattern.test(result.error))) {
		throw new Error(
			`${what}\n  the connection failed, but not with an error this probe recognises as a ` +
				`rejected handshake, so it proves nothing about the peer allowlist. If this is a ` +
				`legitimate rejection shape, add it to HANDSHAKE_REJECTED_PATTERNS with a note ` +
				`saying which stack produces it.\n  error: ${result.error}`,
		);
	}
}

/** Asserts the peer completed the handshake and answered with `status`. */
export function assertAnswered(
	result: MTLSProbeResult,
	status: number,
	what: string,
): void {
	if (result.kind === "refused") {
		throw new Error(
			`${what}\n  expected: HTTP ${status}\n  actual:   the connection was refused: ${result.error}`,
		);
	}
	if (result.kind === "timeout") {
		throw new Error(
			`${what}\n  expected: HTTP ${status}\n  actual:   no answer within ${result.afterMs}ms. ` +
				`The handshake may have succeeded; the listener did not reply.`,
		);
	}
	if (result.status !== status) {
		throw new Error(`${what}\n  expected: HTTP ${status}\n  actual:   ${result.statusLine}`);
	}
}

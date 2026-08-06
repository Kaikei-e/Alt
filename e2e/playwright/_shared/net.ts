import type { APIRequestContext } from "@playwright/test";

/**
 * Transport-level assertions: proving a port is **closed**, not merely
 * guarded.
 *
 * This is the one thing the Hurl suites could not express. Hurl treats an
 * entry that fails to reach the server as a run failure, full stop, so the
 * split-topology suites (alt-data-hub, alt-harvester) had to invert the
 * polarity outside the test framework — a probe file asserting `HTTP *`, a
 * shell wrapper demanding Hurl exit code 3, and a comment explaining that a
 * parse error must not be mistaken for a refusal
 * (e2e/hurl/_lib/assert-transport-refused.sh).
 *
 * In Playwright the fact is directly expressible: `APIRequestContext` rejects
 * on a transport error and resolves on *any* HTTP status, so "the connection
 * must not be established" is a normal assertion with a normal failure
 * message. The distinction the shell helper worked so hard to preserve — a
 * broken probe must never read as a pass — survives here as an explicit match
 * on the error text: an unrecognised error is a failure, not a refusal.
 */

/**
 * Node/undici surfaces a refused or reset connection through these. Matching
 * the text rather than accepting any rejection is deliberate: a typo in the
 * URL, an aborted request or an out-of-memory error would otherwise all read
 * as "the boundary is closed".
 */
const REFUSED_PATTERNS: readonly RegExp[] = [
	/ECONNREFUSED/i,
	/ECONNRESET/i,
	/EHOSTUNREACH/i,
	/ENETUNREACH/i,
	/socket hang up/i,
	/connect(ion)? (refused|reset)/i,
];

/**
 * A TLS listener that rejects the handshake. `EPROTO` is what Node reports
 * for an alert during the handshake; the alert names appear when OpenSSL
 * passes its own diagnostic through.
 */
const TLS_REJECTED_PATTERNS: readonly RegExp[] = [
	/EPROTO/i,
	/SSL routines/i,
	/alert (handshake failure|certificate required|bad certificate|unknown ca)/i,
	/self.signed certificate/i,
	/unable to verify the first certificate/i,
	/certificate required/i,
	/socket hang up/i,
	/ECONNRESET/i,
];

/** Short by design: a refused connection is instant, so a slow one is news. */
const PROBE_TIMEOUT_MS = 5_000;

async function probe(
	api: APIRequestContext,
	url: string,
): Promise<{ answered: number } | { rejected: string }> {
	try {
		const response = await api.get(url, { timeout: PROBE_TIMEOUT_MS, failOnStatusCode: false });
		return { answered: response.status() };
	} catch (error) {
		return { rejected: error instanceof Error ? error.message : String(error) };
	}
}

function matches(message: string, patterns: readonly RegExp[]): boolean {
	return patterns.some((pattern) => pattern.test(message));
}

/**
 * Asserts nothing is listening on `url` — no process bound, or the peer
 * resets immediately.
 *
 * `what` is prose describing the boundary ("alt-harvester exposes no
 * user-facing Connect listener on :9101"); it is what a reader sees when the
 * assertion fails, so make it a claim about the system, not about the port.
 */
export async function expectConnectionRefused(
	api: APIRequestContext,
	url: string,
	what: string,
): Promise<void> {
	const result = await probe(api, url);

	if ("answered" in result) {
		throw new Error(
			`${what}\n  expected: the connection to ${url} is refused\n` +
				`  actual:   the server answered with ${result.answered} — this boundary is open`,
		);
	}
	if (!matches(result.rejected, REFUSED_PATTERNS)) {
		throw new Error(
			`${what}\n  ${url} failed, but not with a connection-refused error, so this ` +
				`proves nothing about the boundary.\n  error: ${result.rejected}`,
		);
	}
}

/**
 * Asserts `url` speaks TLS but rejects *this* client — the mutual-TLS case:
 * no client certificate, or a certificate whose subject is not in the
 * server's allowed-peers list.
 *
 * Distinct from `expectConnectionRefused` because the two failures mean
 * opposite things: nothing bound is a topology fact, a rejected handshake is
 * an authorization fact. A helper that accepted either would let a
 * mutual-TLS listener silently disappear and still report green.
 */
export async function expectTlsHandshakeRejected(
	api: APIRequestContext,
	url: string,
	what: string,
): Promise<void> {
	const result = await probe(api, url);

	if ("answered" in result) {
		throw new Error(
			`${what}\n  expected: the TLS handshake with ${url} is rejected\n` +
				`  actual:   the server answered with ${result.answered} — it accepted this client`,
		);
	}
	if (!matches(result.rejected, TLS_REJECTED_PATTERNS)) {
		throw new Error(
			`${what}\n  ${url} failed, but not with a TLS handshake error, so this proves ` +
				`nothing about who the listener admits.\n  error: ${result.rejected}`,
		);
	}
}

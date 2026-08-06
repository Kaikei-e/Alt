import { expect, test } from "../src/fixtures.js";
import { callUnary } from "../../_shared/connect.js";
import { expectConnectionRefused } from "../../_shared/net.js";
import { expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { env, Procedure } from "../src/env.js";
import { articleCreatedEvent, publishRequest } from "../src/events.js";
import { int64, publishResponseSchema, streamInfoSchema } from "../src/schemas.js";

/**
 * Listener topology and the security posture that follows from it — new
 * coverage, and the part of this service the Hurl suite explicitly could not
 * express.
 *
 * mq-hub is one process with one listener. `config.go` reads a single
 * `CONNECT_PORT`; `main.go` builds one `http.Server` and hangs the Connect
 * prefix, `/health` and `/metrics` off its mux. There is no TLS anywhere in
 * the binary, no auth interceptor, and `middleware.PeerIdentityMiddleware` is
 * unwired dead code whose own package comment says so — which main.go
 * announces at startup with a `peer_identity_disabled` warning, exactly as
 * CLAUDE.md rule 8 requires of a control that is off.
 *
 * The consequence is that **network reachability is the entire access
 * control** for a service that can write to every event stream in Alt. That is
 * a deliberate posture, not an oversight, but it is one nothing was asserting.
 * The tests below pin it in both directions: what answers, and what must not.
 *
 * `e2e/hurl/mq-hub/README.md` listed mTLS peer identity under "out of scope"
 * with the note "if/when it becomes required, add a scenario here". When it
 * does, these tests fail — which is the intended signal.
 */
test.describe("listener topology", () => {
	test("everything is served from the one Connect port", { tag: "@smoke" }, async ({ api }) => {
		// The positive control that gives the negatives below their meaning.
		// All three surfaces share a mux, so if this port were down the
		// "nothing answers on 9110" assertion would pass for the wrong reason.
		await expectStatus(await api.get("/health"), 200);
		await expectStatus(await api.get("/metrics"), 200);
		await expectStatus(await callUnary(api, Procedure.healthCheck, {}), 200);
	});

	test("there is no separate operator listener", { tag: "@contract" }, async ({ bare }) => {
		// 9110 is where the ADR-000954 split binaries (alt-backend,
		// alt-harvester, alt-data-hub) publish /health + /metrics, and where
		// observability/prometheus/prometheus.yml scrapes them. mq-hub has not
		// been split and serves both on its RPC port instead.
		//
		// Asserting the absence is what stops a half-finished migration: add
		// the scrape target before the listener and this fails here, rather
		// than showing up as a silently missing series on a dashboard weeks
		// later. Hurl could not express this at all — a connection it cannot
		// establish is a run failure, which is why the split-topology Hurl
		// suites needed a shell wrapper demanding exit code 3.
		await expectConnectionRefused(
			bare,
			`${env.opsAbsentURL}/health`,
			"mq-hub serves /health and /metrics on its Connect port; it exposes no " +
				"separate operator listener on 9110",
		);
	});

	test("an unregistered path 404s", { tag: "@contract" }, async ({ bare }) => {
		// http.ServeMux has patterns for exactly `/health`, `/metrics` and the
		// Connect prefix — no root pattern — so anything else is a 404 rather
		// than a fallthrough. Cheap fence against a future catch-all handler
		// (a debug route, a pprof mount) landing on a listener whose only
		// access control is the network.
		await expectStatus(await bare.get("/"), 404);
		await expectStatus(await bare.get("/debug/pprof/"), 404);
		await expectStatus(await bare.get("/admin"), 404);
	});
});

test.describe("access control posture", () => {
	test("no credential is required to write to a stream", { tag: "@authz" }, async ({ bare, stream }) => {
		// Pins current behaviour. Anyone who can open a TCP connection to
		// :9500 can publish any event to any stream — including
		// `alt:events:articles`, which pre-processor and search-indexer consume
		// as fact. In the compose topology that is contained by the network
		// (mq-hub publishes no host port outside the staging slice and sits on
		// an `internal: true` network), and nothing else.
		//
		// This test failing means an auth boundary was added — good news, and a
		// signal to write the positive assertions that replace it.
		await expectJsonStatus(
			await callUnary(bare, Procedure.publish, publishRequest(stream, articleCreatedEvent())),
			200,
			publishResponseSchema,
		);
	});

	test("a spoofed peer-identity header changes nothing", { tag: "@authz" }, async ({ bare, stream }) => {
		// `X-Alt-Peer-Identity` is the header PeerIdentityMiddleware *sets*
		// from a verified client certificate's CommonName before calling the
		// next handler (peer_identity.go:68). Because that middleware is not in
		// any chain, the header is untrusted input on this listener — and
		// nothing downstream reads it, which is what makes the current state
		// safe rather than merely undefended.
		//
		// The claim is a *comparison*, so the test has to make both calls. One
		// publish carrying the header and a `success: true` assertion is the
		// preceding test with an extra header attached, and it cannot fail —
		// publishResponseSchema already pins `success` to the literal. Two
		// publishes onto the same per-test stream, differing only in the
		// header, is what makes "changes nothing" observable: same HTTP status,
		// two distinct entries, and both of them actually in the stream.
		//
		// If a future change starts *trusting* the header without an mTLS
		// listener in front to overwrite it, an attacker sets their own
		// identity by typing it — the classic proxy-header-spoofing bug. The
		// first thing such a change does is treat the two requests differently.
		const spoofed = await callUnary(
			bare,
			Procedure.publish,
			publishRequest(stream, articleCreatedEvent()),
			{ headers: { "X-Alt-Peer-Identity": "pre-processor" } },
		);
		const plain = await callUnary(
			bare,
			Procedure.publish,
			publishRequest(stream, articleCreatedEvent()),
		);

		expect(
			spoofed.status(),
			"a client-supplied peer identity changed the outcome of a publish; the header " +
				"is untrusted input on this listener and nothing may read it",
		).toBe(plain.status());

		const spoofedBody = await expectJsonStatus(spoofed, 200, publishResponseSchema);
		const plainBody = await expectJsonStatus(plain, 200, publishResponseSchema);
		expect(spoofedBody.messageId).not.toBe(plainBody.messageId);

		// Both landed. A rejection that still answered 200, or a silent drop of
		// the header-bearing request, would show up here as a length of 1.
		const info = await expectJsonStatus(
			await callUnary(bare, Procedure.getStreamInfo, { stream }),
			200,
			streamInfoSchema,
		);
		expect(
			int64(info.length),
			"both publishes must be in the stream — the header may not gate the write",
		).toBe(2);
	});
});

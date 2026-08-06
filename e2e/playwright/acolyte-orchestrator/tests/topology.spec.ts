import { expectConnectionRefused } from "../../_shared/net.js";
import { expectStatusIn } from "../../_shared/http.js";
import { env } from "../src/env.js";
import { test } from "../src/fixtures.js";

/**
 * Listener topology — new coverage. Hurl could not express any of this.
 *
 * `main.py` builds exactly one Starlette app with exactly two entries in
 * `routes=[...]`: `Route("/health")` and `Mount(asgi_app.path)`. There is no
 * metrics endpoint, no admin surface and no second port. That is a small
 * surface, and its smallness is the security property — `AcolyteService`
 * authenticates nobody (`PEER_IDENTITY_TRUSTED=off`, `PEER_IDENTITY_STRICT`
 * unset in the staging slice), so every reachable route is an unauthenticated
 * route. The comment on the production port publish spells it out:
 * "AcolyteService authenticates nobody on it, so publishing it on every NIC is
 * a second entrance the sidecar cannot see" (compose/acolyte.yaml:124-128).
 *
 * These tests fence that surface. Anything new answering on :8090, or anything
 * answering on the sidecar's port from inside this container, is a finding.
 */

test.describe("nothing else is listening", () => {
	test("the mTLS sidecar port is closed in this slice @authz", async ({ rest }) => {
		// In production, pki-agent-acolyte-orchestrator runs with
		// `network_mode: "service:acolyte-orchestrator"` (compose/pki.yaml:371)
		// and terminates mutual TLS on :9443 inside this container's network
		// namespace, proxying to 127.0.0.1:8090. The staging slice runs no
		// pki-agent, so nothing may be bound there.
		//
		// This is the only assertion in the suite that can catch the
		// orchestrator process opening a second listener of its own — a
		// uvicorn started with a TLS config, say, or a debug server. The
		// distinction `_shared/net.ts` preserves matters here: a *refused*
		// connection means nothing is bound, which is the claim. A rejected TLS
		// handshake would mean something is bound and merely picky, and that
		// would be news.
		await expectConnectionRefused(
			rest,
			`${env.mtlsSidecarURL}/health`,
			"acolyte-orchestrator binds only :8090; :9443 belongs to the pki-agent " +
				"sidecar (compose/pki.yaml:369-400), which this slice does not run",
		);
	});
});

/**
 * Paths that must not resolve on :8090.
 *
 * None of these is registered in `main.py`. Each is here because it is a route
 * some *other* Alt service has, and a copy-paste or a future middleware could
 * plausibly bring it into this one — where it would be reachable by anyone who
 * can open a socket, since nothing on this listener authenticates a caller.
 */
const ABSENT_REST_PATHS = [
	// Prometheus scrapes several Alt services at /metrics. This one is scraped
	// through the rask log pipeline instead (OTEL_EXPORTER_OTLP_ENDPOINT in
	// compose/acolyte.yaml:117-119), so a /metrics here would be a new,
	// unauthenticated surface exposing run counts and model names.
	"/metrics",
	// The Go services' operator probe. The public health route here is
	// /health — asserted positively in tests/health.spec.ts — and there is no
	// versioned twin.
	"/v1/health",
	// Starlette serves no index and no docs. FastAPI's defaults (/docs,
	// /openapi.json) would publish the entire RPC surface to any caller; this
	// app is plain Starlette and must stay that way.
	"/docs",
	"/openapi.json",
	"/",
] as const;

test.describe("the REST surface is /health and nothing else", () => {
	for (const path of ABSENT_REST_PATHS) {
		test(`GET ${path} does not resolve @contract`, async ({ rest }) => {
			// A band with two members, both meaning "this is not a route of this
			// application":
			//   - **404** — `Mount(asgi_app.path)` handed the path to the Connect
			//     app, which has no endpoint by that name (or Starlette's router
			//     found no match at all).
			//   - **405** — the Connect layer rejected the *method* first: every
			//     procedure on this service is unary POST, so a GET can never be
			//     served regardless of the path.
			// Everything else is a finding. A **200** means a route was added to
			// an unauthenticated listener. A **401/403** means a route exists and
			// is merely guarded — which on a listener whose access control is
			// entirely "who can open a socket" is barely a distinction, and is
			// certainly not the "this surface is not here" claim being made.
			await expectStatusIn(await rest.get(path), [404, 405]);
		});
	}
});

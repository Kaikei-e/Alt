import { expect, extractTags, test } from "../src/fixtures.js";
import { expectStatusIn } from "../../_shared/http.js";
import { expectConnectionRefused } from "../../_shared/net.js";
import { env } from "../src/env.js";

/**
 * The authentication boundary — entirely new coverage.
 *
 * The Hurl suite deferred all of this ("authenticated endpoints… staging
 * currently runs a no-op auth fallback"). That description is no longer true
 * of the code, and the direction it moved is exactly the kind of thing an E2E
 * suite exists to hold in place:
 *
 *   - `alt_auth.client` is **not** a dependency of this service
 *     (tag-generator/app/pyproject.toml), so `_ALT_AUTH_AVAILABLE` is False
 *     and the fallback `require_auth` decorator refuses every call to the
 *     routes it wraps instead of fabricating an anonymous `UserContext`
 *     (auth_service.py:52-93). That is `.claude/rules/di-wiring.md` — a
 *     missing auth module must fail closed, not disable authentication.
 *
 *   - `/api/v1/extract-tags` has, by contrast, **no** authentication at all on
 *     :9400. `verify_service_token` is a documented no-op and says so in its
 *     own docstring (auth_service.py:524-533): reachability is the entire
 *     control. The tests below pin that as current behaviour, because an
 *     assertion that fails when someone adds real authentication is the
 *     correct signal — not because the state of affairs is desirable.
 */

/** The two routes wrapped by the fail-closed `require_auth` decorator. */
const AUTHENTICATED_ROUTES = [
	{ method: "POST" as const, path: "/api/v1/generate-tags", source: "auth_service.py:453" },
	{ method: "GET" as const, path: "/api/v1/user-preferences", source: "auth_service.py:511" },
];

test.describe("authenticated routes fail closed", () => {
	for (const { method, path, source } of AUTHENTICATED_ROUTES) {
		test(`${method} ${path} never serves data anonymously`, {
			tag: "@authz",
		}, async ({ api }) => {
			const response =
				method === "POST"
					? await api.post(path, { data: { article_id: "x", title: "t", content: "c" } })
					: await api.get(path);

			// Mounted, first. A 404 would mean the route vanished, and every
			// other assertion in this test would then pass for the wrong
			// reason — "you cannot get data from it" is trivially true of a
			// route that does not exist. tests/route-surface.spec.ts proves
			// registration from the OpenAPI document; this repeats the claim
			// at the point where it is load-bearing.
			expect(
				response.status(),
				`${path} is registered at ${source}; a 404 means it is gone, not that the ` +
					`caller was rejected`,
			).not.toBe(404);

			// The safety property: no 2xx, ever, without a credential.
			expect(
				response.status(),
				`${path} answered ${response.status()} to an unauthenticated caller`,
			).toBeGreaterThanOrEqual(400);

			// Two answers are correct here, for two different reasons, and both
			// are refusals:
			//
			//   503 — the fail-closed `require_auth` wrapper ran and rejected
			//         the call because `alt_auth.client` is not installed.
			//   422 — FastAPI never got that far. The wrapper carries the
			//         wrapped function's signature through `@wraps`, so the
			//         `user_context: UserContext` parameter is still visible to
			//         FastAPI as a request-body model, and request validation
			//         runs before the endpoint callable does.
			//
			// Anything outside the pair — and in particular a 200 — would mean
			// the route is being served without authentication.
			await expectStatusIn(response, [422, 503]);
		});

		test(`${method} ${path} is not unlocked by a forged peer identity`, {
			tag: "@authz",
		}, async ({ playwright }) => {
			// `X-Alt-Peer-Identity` is set by the nginx mTLS sidecar and is the
			// only caller identity this service has. `PeerIdentityMiddleware`
			// honours it under two conditions — `PEER_IDENTITY_TRUSTED=on` *and*
			// a loopback transport peer — and this slice satisfies neither
			// (compose.staging.yaml sets it to `off`, and the suite connects
			// over the bridge network). A header written by the client is
			// therefore discarded before it reaches any handler
			// (peer_identity.py:88-96).
			//
			// This is the forgery negative that makes the whole peer-identity
			// design meaningful: without it, "authenticated" would mean
			// "willing to type a header".
			const forged = await playwright.request.newContext({
				baseURL: env.baseURL,
				extraHTTPHeaders: {
					"Content-Type": "application/json",
					"X-Alt-Peer-Identity": "recap-worker",
				},
			});
			try {
				const response =
					method === "POST"
						? await forged.post(path, { data: { article_id: "x", title: "t", content: "c" } })
						: await forged.get(path);
				expect(
					response.status(),
					`${path} served a caller whose only credential was a self-written header`,
				).toBeGreaterThanOrEqual(400);
			} finally {
				await forged.dispose();
			}
		});
	}
});

test.describe("the plaintext listener's actual trust model", () => {
	test("/api/v1/extract-tags authenticates nobody on :9400", {
		tag: "@authz",
	}, async ({ api }) => {
		// Pinning current behaviour, deliberately. `verify_service_token` is a
		// no-op whose docstring states that reachability — compose binding the
		// port to loopback — is what keeps :9400 closed, and that the function
		// must not be read as a second control. An E2E suite that quietly
		// assumed the endpoint were protected would be the exact
		// misunderstanding the docstring warns against.
		//
		// When this endpoint grows real authentication, this test fails. That
		// is the intended signal, and the fix is to move the assertion to the
		// authenticated-routes block above.
		const body = await extractTags(api, {
			title: "Anonymous callers are served",
			content:
				"This request carries no credential of any kind: no bearer token, no client " +
				"certificate, and no peer identity header. It is served anyway.",
		});
		expect(body.success).toBe(true);
	});

	test("nothing answers on the mTLS sidecar port in this slice", {
		tag: "@authz",
	}, async ({ api }) => {
		// The other half of the claim above. In production tag-generator sits
		// behind an nginx sidecar on :9443 that terminates mutual TLS and is
		// the only thing entitled to set `X-Alt-Peer-Identity`; this staging
		// slice deliberately runs no sidecar, which is why `MTLS_ENFORCE=false`
		// and `PEER_IDENTITY_TRUSTED=off` are correct here and why the suite
		// speaks plaintext.
		//
		// Asserting the port is *closed* turns that from an unstated assumption
		// into a fact the suite checks. If a sidecar ever appears in this slice
		// without the trust flags moving with it, the peer-identity assertions
		// above would start passing for a reason that has nothing to do with
		// what they claim to test.
		await expectConnectionRefused(
			api,
			env.tlsSidecarURL,
			"this staging slice runs no nginx mTLS sidecar for tag-generator, so :9443 must " +
				"have nothing bound to it",
		);
	});
});

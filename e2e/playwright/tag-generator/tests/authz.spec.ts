import type { APIRequestContext, APIResponse } from "@playwright/test";
import { expect, extractTags, test } from "../src/fixtures.js";
import { expectStatus } from "../../_shared/http.js";
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

/**
 * The two routes wrapped by the fail-closed `require_auth` decorator, each
 * with a body FastAPI will actually accept.
 *
 * The body is the whole difficulty of this file. `@require_auth` is applied
 * *under* `@app.post` / `@app.get`, and its wrappers use `@wraps`, which sets
 * `__wrapped__`; FastAPI's `get_typed_signature` calls `inspect.signature`
 * with the default `follow_wrapped=True`, so FastAPI builds its dependant from
 * the **undecorated** signature. Both endpoints therefore still declare their
 * `UserContext` parameter as a request body, and a request that does not
 * satisfy it is rejected by validation with a 422 *before the decorator ever
 * runs* — i.e. before the thing these tests exist to check.
 *
 *   - `generate_tags_endpoint(request: TagGenerationRequest,
 *     user_context: UserContext)` has two non-scalar params, so FastAPI embeds
 *     them under their parameter names (auth_service.py:453-455).
 *   - `get_user_preferences(user_context: UserContext)` has one, so the bare
 *     object *is* the body (auth_service.py:511-512).
 *
 * Every `UserContext` field carries a default (auth_service.py:33-37), so `{}`
 * validates.
 */
const AUTHENTICATED_ROUTES = [
	{
		method: "POST" as const,
		path: "/api/v1/generate-tags",
		source: "auth_service.py:453",
		body: {
			request: { article_id: "x", title: "t", content: "c" },
			user_context: {},
		} as Record<string, unknown>,
	},
	{
		method: "GET" as const,
		path: "/api/v1/user-preferences",
		source: "auth_service.py:511",
		body: {} as Record<string, unknown>,
	},
];

type AuthenticatedRoute = (typeof AUTHENTICATED_ROUTES)[number];

/** Issues the route's request on an arbitrary client, body included. */
function callRoute(client: APIRequestContext, route: AuthenticatedRoute): Promise<APIResponse> {
	return route.method === "POST"
		? client.post(route.path, { data: route.body })
		: client.get(route.path, { data: route.body });
}

/**
 * The one string only the fail-closed branch emits
 * (auth_service.py:71-73). Matching on it — rather than on the status alone —
 * is what separates "the wrapper refused" from "something else answered 503",
 * e.g. the readiness 503 that `/api/v1/extract-tags` raises.
 */
const FAIL_CLOSED_DETAIL = "Authentication is unavailable";

test.describe("authenticated routes fail closed", () => {
	for (const route of AUTHENTICATED_ROUTES) {
		const { method, path, source } = route;

		test(`${method} ${path} never serves data anonymously`, {
			tag: "@authz",
		}, async ({ api }) => {
			const response = await callRoute(api, route);

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

			// Exactly 503, not a band. A 422 here would mean the request never
			// reached the decorator, so the test would be reporting green on a
			// body-shape mismatch of its own making — and would keep doing so
			// if someone swapped the fail-closed wrapper for the anonymous
			// `UserContext` no-op that .claude/rules/di-wiring.md forbids.
			await expectStatus(response, 503);

			// The detail string is the assertion that discriminates the
			// *reason*: a 503 from anywhere else in the app (a readiness check,
			// a proxy) would satisfy the status and not this.
			expect(
				await response.text(),
				`${path} answered 503 for some reason other than the fail-closed auth wrapper`,
			).toContain(FAIL_CLOSED_DETAIL);
		});

		test(`${method} ${path} answers a forged peer identity exactly as it answers an anonymous caller`, {
			tag: "@authz",
		}, async ({ api, playwright }) => {
			// `X-Alt-Peer-Identity` is set by the nginx mTLS sidecar and is the
			// only caller identity this service has. `PeerIdentityMiddleware`
			// honours it under two conditions — `PEER_IDENTITY_TRUSTED=on` *and*
			// a loopback transport peer — and this slice satisfies neither
			// (compose.staging.yaml:468-469 sets it to `off`, and the suite
			// connects over the bridge network), so the header is blanked at
			// peer_identity.py:88-94.
			//
			// What this slice can and cannot prove, stated plainly: the
			// middleware runs with `strict=False` (auth_service.py:445) and no
			// handler reads `request.state.peer_identity`, so whether the
			// header was honoured or discarded is *not directly observable in
			// the response*. Proving "discarded" would need a stack with
			// `strict=True`. What is observable, and what this test asserts, is
			// the weaker but still falsifiable claim: writing the header
			// changes nothing about the answer. If it ever does — a middleware
			// that starts trusting the client-written value while `strict` is
			// on would answer the forged request differently from the plain one
			// — this fails.
			const forged = await playwright.request.newContext({
				baseURL: env.baseURL,
				extraHTTPHeaders: {
					"Content-Type": "application/json",
					"X-Alt-Peer-Identity": "recap-worker",
				},
			});
			try {
				const forgedResponse = await callRoute(forged, route);
				const plainResponse = await callRoute(api, route);

				expect(
					forgedResponse.status(),
					`${path} answered a self-written X-Alt-Peer-Identity with a different status ` +
						`than it answered the same request without one`,
				).toBe(plainResponse.status());
				expect(
					await forgedResponse.text(),
					`${path} answered a self-written X-Alt-Peer-Identity with a different body ` +
						`than it answered the same request without one`,
				).toBe(await plainResponse.text());

				// And the answer both of them got is still the refusal — the
				// equality above would also hold if both had been served.
				await expectStatus(forgedResponse, 503);
				expect(await forgedResponse.text()).toContain(FAIL_CLOSED_DETAIL);
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

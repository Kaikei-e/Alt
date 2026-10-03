import type { APIRequestContext, APIResponse } from "@playwright/test";
import { expect, extractTags, test } from "../src/fixtures.js";
import { expectStatus } from "../../_shared/http.js";
import { expectTlsHandshakeRejected } from "../../_shared/net.js";
import { clientCertificates } from "../../_shared/client-auth.js";
import { env } from "../src/env.js";

/** The verified peer gate precedes the endpoint's fail-closed user-auth decorator. */

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
			// A supplied header cannot replace the authenticated certificate identity.
			const forged = await playwright.request.newContext({
				baseURL: env.baseURL,
				clientCertificates: clientCertificates(env.baseURL, env.clientCert, env.clientKey),
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

test.describe("verified TLS peer boundary", () => {
	test("/api/v1/extract-tags serves the permitted certificate holder", {
		tag: "@authz",
	}, async ({ api }) => {
		// This client presents an allowed leaf; transport authorization succeeds.
		const body = await extractTags(api, {
			title: "Permitted certificate callers are served",
			content:
				"This request carries the permitted client certificate and receives tags.",
		});
		expect(body.success).toBe(true);
	});

	test("TLS requires a client certificate", {
		tag: "@authz",
	}, async ({ api, playwright }) => {
		// The positive control proves a live TLS listener before the rejection probe.
		await expectStatus(await api.get("/health"), 200);
		const anonymous = await playwright.request.newContext();
		try {
			await expectTlsHandshakeRejected(anonymous, `${env.baseURL}/health`, "tag TLS requires a client certificate");
		} finally { await anonymous.dispose(); }
	});
	test("plaintext and forged peer headers cannot serve business requests", { tag: "@authz" }, async ({ playwright }) => {
		const plain = await playwright.request.newContext({ baseURL: env.plaintextURL });
		try {
			for (const headers of [{}, { "X-Alt-Peer-Identity": "alt-backend" }]) {
				await expectStatus(await plain.post("/api/v1/extract-tags", { headers, data: { title: "denied", content: "denied" } }), 401);
			}
		} finally { await plain.dispose(); }
	});
	test("a valid CA certificate with an unauthorized CN is rejected", { tag: "@authz" }, async ({ api, playwright }) => {
		await expectStatus(await api.get("/health"), 200);
		const denied = await playwright.request.newContext({ clientCertificates: clientCertificates(env.baseURL, env.deniedCert, env.deniedKey) });
		try {
			await expectTlsHandshakeRejected(denied, `${env.baseURL}/health`, "tag rejects an unauthorized certificate CN");
		} finally { await denied.dispose(); }
	});
});

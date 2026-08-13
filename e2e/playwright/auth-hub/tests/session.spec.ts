import { test, expect } from "../src/fixtures.js";
import { expectJson, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { INVALID_SESSION_VALUE } from "../src/env.js";
import { echoErrorSchema, sessionResponseSchema } from "../src/schemas.js";

/**
 * `/session` — the SPA's session bootstrap. Ports `05` and `06`.
 *
 * Where `/validate` speaks to nginx in headers, `/session` speaks to the
 * browser in JSON: `alt-frontend-sv` calls it on load to decide whether it is
 * signed in and who it is signed in as. `05-session-happy.hurl` asserted five
 * JSONPaths, one of which was `jsonpath "$.session.id" exists` — which is
 * satisfied by `null`. `sessionResponseSchema` replaces the whole set with one
 * assertion over the whole envelope; see src/schemas.ts for why `role` is an
 * enum and `lastLoginAt` is required despite its `omitempty` tag.
 */

test.describe("session — authenticated", () => {
	test("GET /session returns the full session envelope @smoke", async ({ hub, session }) => {
		const body = await expectJsonStatus(
			await hub.get("/session", { headers: { Cookie: session.cookieHeader } }),
			200,
			sessionResponseSchema,
		);

		expect(body.user.id).toBe(session.userId);
		// Single-tenant: `usecase.GetSession` sets `tenantID = identity.UserID`.
		expect(body.user.tenantId).toBe(session.userId);
		expect(body.user.email).toBe(session.email);
		expect(body.session.active).toBe(true);
	});

	test("GET /session reports the role Kratos actually carries @contract", async ({
		hub,
		session,
	}) => {
		// The staging identity schema forbids extra traits
		// (`e2e/fixtures/auth-hub/kratos/identity.schema.json`, `additionalProperties:
		// false`), so no seeded identity can have a `role` trait and
		// `gateway/kratos.go` must default it to "user". The SPA renders admin
		// affordances off this field, so a handler that started defaulting to
		// "admin" — or echoing an unvalidated trait — is a privilege bug that no
		// status code would reveal.
		const body = await expectJsonStatus(
			await hub.get("/session", { headers: { Cookie: session.cookieHeader } }),
			200,
			sessionResponseSchema,
		);
		expect(body.user.role).toBe("user");
	});

	test("GET /session dates are real instants @contract", async ({ hub, session }) => {
		// `createdAt` comes from the Kratos identity and `lastLoginAt` is
		// `time.Now()` at handler time (session.go:60). Both are `time.Time`, so
		// the failure mode is not a malformed string — the schema already covers
		// that — but the Go zero value, `0001-01-01T00:00:00Z`, which is what a
		// dropped field or an unpopulated cache entry produces and which every
		// RFC 3339 regex happily accepts.
		const body = await expectJsonStatus(
			await hub.get("/session", { headers: { Cookie: session.cookieHeader } }),
			200,
			sessionResponseSchema,
		);

		const createdAt = Date.parse(body.user.createdAt);
		const lastLoginAt = Date.parse(body.user.lastLoginAt);
		expect(Number.isNaN(createdAt), `createdAt ${body.user.createdAt}`).toBe(false);
		expect(Number.isNaN(lastLoginAt), `lastLoginAt ${body.user.lastLoginAt}`).toBe(false);

		// The identity was created by this run's own fixture, so "within the last
		// hour" is a real bound, not a formality; the Go zero value is ~2000 years
		// off and fails it decisively.
		expect(Date.now() - createdAt, "createdAt should be recent").toBeLessThan(3_600_000);
		expect(Date.now() - createdAt, "createdAt should be in the past").toBeGreaterThan(-60_000);
		// `lastLoginAt` is stamped at request time, so it is always "now".
		expect(Math.abs(Date.now() - lastLoginAt), "lastLoginAt should be now").toBeLessThan(
			300_000,
		);
	});

	test("GET /session identifies the session it was given @contract", async ({ hub, session }) => {
		// `session.id` must be Kratos's session UUID, the same value the backend
		// JWT carries as `sid`. It must never be the raw `ory_kratos_session`
		// cookie: that is the bearer credential itself, and this JSON body is
		// something the SPA may log or store.
		//
		// `05-session-happy.hurl` asserted `jsonpath "$.session.id" exists`, which
		// told nobody any of this.
		const body = await expectJsonStatus(
			await hub.get("/session", { headers: { Cookie: session.cookieHeader } }),
			200,
			sessionResponseSchema,
		);
		expect(body.session.id).toBe(session.kratosSessionId);
		expect(body.session.id).not.toBe(session.cookie);
	});

	test("GET /session sets the backend token header @contract", async ({ hub, session }) => {
		// The claim contents are asserted in tests/backend-token.spec.ts; this is
		// the presence check that belongs with the endpoint's own contract.
		const response = await hub.get("/session", { headers: { Cookie: session.cookieHeader } });
		await expectStatus(response, 200);
		expect(response.headers()["x-alt-backend-token"], "X-Alt-Backend-Token").toBeTruthy();
	});

	test("two sessions see only their own identity @authz", async ({
		hub,
		session,
		mintSession,
	}) => {
		// New coverage. The Hurl suite had exactly one identity, so no scenario
		// could have caught a cache that returned the wrong user — the single most
		// damaging failure this service can have, and one that presents as a
		// perfectly ordinary 200.
		const other = await mintSession();

		const mine = await expectJsonStatus(
			await hub.get("/session", { headers: { Cookie: session.cookieHeader } }),
			200,
			sessionResponseSchema,
		);
		const theirs = await expectJsonStatus(
			await hub.get("/session", { headers: { Cookie: other.cookieHeader } }),
			200,
			sessionResponseSchema,
		);

		expect(theirs.user.id).toBe(other.userId);
		expect(theirs.user.email).toBe(other.email);
		expect(mine.user.id).not.toBe(theirs.user.id);
	});
});

test.describe("session — rejected", () => {
	test("/session without a cookie → 401 @authz", async ({ hub }) => {
		// session.go:47-50 short-circuits on `c.Cookie` before the usecase runs.
		const response = await hub.get("/session");
		await expectStatus(response, 401);
		// New: the envelope, not just the status. Echo renders
		// `echo.NewHTTPError` as `{"message": …}` — the SPA branches on that key,
		// and `/csrf` answers the same situation with `{"error": …}` instead
		// (tests/csrf.spec.ts). Which key appears is a contract even though the
		// phrasing is not.
		await expectJson(response, echoErrorSchema);
	});

	test("/session with an unknown session value → 401 @authz", async ({ hub }) => {
		// New: `06-session-unauth.hurl` only covered the missing-cookie
		// short-circuit, so the Kratos-negative branch of /session — the one that
		// actually calls out to the identity provider — had no coverage at all.
		const response = await hub.get("/session", {
			headers: { Cookie: `ory_kratos_session=${INVALID_SESSION_VALUE}` },
		});
		await expectStatus(response, 401);
		await expectJson(response, echoErrorSchema);
	});

	test("a rejected /session emits no backend token @authz", async ({ hub }) => {
		// `c.Response().Header().Set("X-Alt-Backend-Token", …)` runs only after
		// `uc.Execute` succeeds. A 401 carrying a token would hand an
		// unauthenticated caller a credential alt-backend accepts.
		const response = await hub.get("/session");
		await expectStatus(response, 401);
		expect(response.headers()["x-alt-backend-token"]).toBeUndefined();
	});

	test("POST /session is not routed @contract", async ({ hub }) => {
		// `e.GET("/session", ...)`.
		await expectStatus(await hub.post("/session"), 405);
	});
});

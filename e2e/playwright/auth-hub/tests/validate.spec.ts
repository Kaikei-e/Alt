import { test, expect } from "../src/fixtures.js";
import {
	expectHeader,
	expectJson,
	expectNoHeader,
	expectStatus,
} from "../../_shared/http.js";
import { INVALID_SESSION_VALUE } from "../src/env.js";
import { echoErrorSchema } from "../src/schemas.js";

/**
 * `/validate` — nginx's `auth_request` target, and therefore the single most
 * load-bearing endpoint in the platform. Ports `02`, `03` and `04`.
 *
 * nginx forwards the four `X-Alt-*` response headers upstream and discards the
 * body, so the *headers* are the entire product of this endpoint. Everything
 * downstream — alt-backend's tenant scoping, its JWT verification, the SPA's
 * notion of who is signed in — is a function of what this handler sets.
 *
 * The negatives matter more than the positive. `auth_request` treats any 2xx
 * as "let it through", so a 401 that still emitted `X-Alt-User-Id` would be a
 * complete authentication bypass the moment someone changed a status code —
 * and no assertion in the Hurl suite looked at the negative responses' headers
 * at all. Those assertions are new here.
 */

/** Compact JWS: three base64url segments, the first two starting `ey` (`{"`). */
const JWS_PATTERN = /^ey[A-Za-z0-9_-]+\.ey[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$/;

/** Every header `validate.go` sets on success — and must not set otherwise. */
const IDENTITY_HEADERS = [
	"X-Alt-User-Id",
	"X-Alt-Tenant-Id",
	"X-Alt-User-Email",
	"X-Alt-Backend-Token",
] as const;

test.describe("validate — authenticated", () => {
	test("GET /validate returns the four identity headers @smoke", async ({ hub, session }) => {
		const response = await hub.get("/validate", {
			headers: { Cookie: session.cookieHeader },
		});
		await expectStatus(response, 200);

		expectHeader(response, "X-Alt-User-Id", session.userId);
		// Single-tenant architecture: validate.go sets both from the same
		// `identity.UserID`. Asserting the equality rather than a fixed fixture id
		// is what keeps this test honest when tenancy is eventually decoupled —
		// it will fail here, in the one place that documents the assumption.
		expectHeader(response, "X-Alt-Tenant-Id", session.userId);
		expectHeader(response, "X-Alt-User-Email", session.email);

		const token = response.headers()["x-alt-backend-token"];
		expect(token, "X-Alt-Backend-Token").toMatch(JWS_PATTERN);
	});

	test("GET /validate answers with an empty body @contract", async ({ hub, session }) => {
		// `c.NoContent(http.StatusOK)`. nginx's auth_request subrequest discards
		// the body, so any body here is wasted bytes on the hot path of every
		// single request into the platform — and a handler that started returning
		// the identity as JSON would be leaking it to a surface that has no
		// business seeing it.
		const response = await hub.get("/validate", {
			headers: { Cookie: session.cookieHeader },
		});
		await expectStatus(response, 200);
		expect(await response.text()).toBe("");
	});

	test("GET /validate re-issues the token on every call @contract", async ({ hub, session }) => {
		// The second call is a session-cache hit (CACHE_TTL=5m), which takes a
		// completely different branch through `usecase.ValidateSession` — it
		// rebuilds the identity from `domain.CachedSession` instead of from
		// Kratos. `12-validate-jwt-shape.hurl` silently depended on this working
		// ("cache is warm from scenario 02; the header is re-emitted every call")
		// but never asserted it.
		//
		// The regression this catches is the cache dropping a field: the caching
		// path used to lose `Role`, which downgraded an admin's JWT to "user" on
		// every request after the first (see the comment in
		// usecase/validate_session.go). Comparing the two responses field by field
		// is what makes that visible from outside.
		const first = await hub.get("/validate", { headers: { Cookie: session.cookieHeader } });
		await expectStatus(first, 200);
		const second = await hub.get("/validate", { headers: { Cookie: session.cookieHeader } });
		await expectStatus(second, 200);

		for (const name of ["X-Alt-User-Id", "X-Alt-Tenant-Id", "X-Alt-User-Email"] as const) {
			expect(second.headers()[name.toLowerCase()], `${name} on the cache-hit call`).toBe(
				first.headers()[name.toLowerCase()],
			);
		}
		expect(second.headers()["x-alt-backend-token"], "X-Alt-Backend-Token").toMatch(
			JWS_PATTERN,
		);
	});
});

test.describe("validate — rejected", () => {
	test("/validate without a cookie → 401 @authz", async ({ hub }) => {
		// `c.Cookie("ory_kratos_session")` returns http.ErrNoCookie and the
		// handler short-circuits before any Kratos round-trip
		// (validate.go:26-29). Distinct from the case below, which does reach
		// Kratos.
		const response = await hub.get("/validate");
		await expectStatus(response, 401);
		await expectJson(response, echoErrorSchema);
	});

	test("/validate without a cookie leaks no identity headers @authz", async ({ hub }) => {
		// The bypass fence. nginx `auth_request` forwards whatever `X-Alt-*`
		// headers come back on a 2xx and drops the request otherwise — but an
		// nginx config that maps the headers unconditionally, or a future 200-with-
		// `X-Accel` design, turns a leaked header into an unauthenticated identity
		// assertion. validate.go only sets these after `uc.Execute` succeeds;
		// this is the outside-in proof of that.
		const response = await hub.get("/validate");
		await expectStatus(response, 401);
		for (const name of IDENTITY_HEADERS) {
			expectNoHeader(response, name);
		}
	});

	test("/validate with a Cookie header that has no session → 401 @authz", async ({ hub }) => {
		// A cookie jar full of unrelated cookies (an analytics id, a locale) is
		// the ordinary state of a signed-out browser. `c.Cookie` still returns
		// ErrNoCookie, so this must take the same short-circuit — the presence of
		// *a* Cookie header must not be mistaken for the presence of *the* cookie.
		// (`/csrf` inspects the raw header instead and behaves differently; see
		// tests/csrf.spec.ts.)
		const response = await hub.get("/validate", {
			headers: { Cookie: "ajs_anonymous_id=abc; locale=ja-JP" },
		});
		await expectStatus(response, 401);
		for (const name of IDENTITY_HEADERS) {
			expectNoHeader(response, name);
		}
	});

	test("/validate with an unknown session value → 401 @authz", async ({ hub }) => {
		// Cookie parsing succeeds, the cache misses, the gateway calls Kratos
		// whoami, Kratos answers 401, `domain.ErrAuthFailed` bubbles up and
		// error_mapper.go maps it to 401. The Kratos-negative branch — unlike the
		// two above, which never leave auth-hub.
		//
		// The status is also what separates it from a *broken* Kratos: the same
		// call against an unreachable identity provider answers 502
		// (ErrKratosUnavailable). setup/global-setup.ts uses exactly that
		// distinction as its readiness gate.
		const response = await hub.get("/validate", {
			headers: { Cookie: `ory_kratos_session=${INVALID_SESSION_VALUE}` },
		});
		await expectStatus(response, 401);
		await expectJson(response, echoErrorSchema);
		for (const name of IDENTITY_HEADERS) {
			expectNoHeader(response, name);
		}
	});

	test("/validate with a revoked session → 401 @authz", async ({ hub, kratosAdmin, mintSession }) => {
		// New coverage: the Hurl suite only ever presented a string that was never
		// a session. This presents a real, Kratos-issued session that has since
		// been invalidated — the shape of an actual sign-out or a compromised
		// credential being revoked.
		//
		// The session is minted for this test alone and never presented to
		// auth-hub before revocation, and that ordering is the whole test. auth-hub
		// caches validated sessions for CACHE_TTL=5m keyed on the cookie value
		// (infrastructure/cache/session_cache.go), so a single /validate before the
		// revoke would make this pass with a 200 for the next five minutes. Using
		// the worker-scoped `session` fixture here would also poison every other
		// test on this worker — hence `mintSession`.
		const doomed = await mintSession();

		const revoked = await kratosAdmin.delete(`/admin/identities/${doomed.userId}/sessions`);
		expect(
			revoked.status(),
			`DELETE /admin/identities/{id}/sessions -> ${revoked.status()}: ` +
				`${(await revoked.text()).slice(0, 300)}`,
		).toBe(204);

		const response = await hub.get("/validate", {
			headers: { Cookie: doomed.cookieHeader },
		});
		await expectStatus(response, 401);
		for (const name of IDENTITY_HEADERS) {
			expectNoHeader(response, name);
		}
	});

	test("POST /validate is not routed @contract", async ({ hub }) => {
		// `e.GET("/validate", ...)`. 405 rather than 404 proves the path exists
		// and only the verb was refused — the same discrimination
		// `expectProcedureMounted` makes for Connect services elsewhere in the
		// fleet, and the reason a DI/wiring failure cannot hide here.
		await expectStatus(await hub.post("/validate"), 405);
	});
});

test.describe("validate — identity provenance", () => {
	test("the reported user id is the Kratos identity id @contract", async ({
		hub,
		kratosAdmin,
		session,
	}) => {
		// Ties auth-hub's answer back to the source of truth rather than to a
		// value this suite happens to have captured. If auth-hub started reporting
		// the *session* id, or a cache key, or the tenant fixture, every
		// header-equality assertion above would still pass because they all
		// compare against the same captured string.
		const response = await hub.get("/validate", {
			headers: { Cookie: session.cookieHeader },
		});
		await expectStatus(response, 200);
		const reported = response.headers()["x-alt-user-id"];

		const identity = await kratosAdmin.get(`/admin/identities/${reported}`);
		await expectStatus(identity, 200);
		const body = (await identity.json()) as { id?: unknown; traits?: { email?: unknown } };
		expect(body.id).toBe(session.userId);
		expect(body.traits?.email).toBe(session.email);
	});
});

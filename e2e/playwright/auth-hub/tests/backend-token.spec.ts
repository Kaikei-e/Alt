import { test, expect } from "../src/fixtures.js";
import type { APIRequestContext } from "@playwright/test";
import { expectStatus } from "../../_shared/http.js";
import { env } from "../src/env.js";
import { audiences, decodeJwt, numberClaim, stringClaim, verifyHS256 } from "../src/jwt.js";

/**
 * The `X-Alt-Backend-Token` HS256 JWT — the port of `12-validate-jwt-shape.hurl`,
 * substantially strengthened.
 *
 * This token is the entire authentication contract between auth-hub and
 * alt-backend. nginx copies it out of the `/validate` response and onto the
 * upstream request; alt-backend verifies it against the same shared secret and
 * derives the caller's subject, tenant and role from its claims. A drift in
 * any one of issuer, audience, signing key or claim name is a total outage of
 * every authenticated endpoint in the platform — and it is invisible to both
 * services' unit tests, because each of them only ever sees its own half.
 *
 * Hurl could reach the claims only as a substring match over the decoded
 * payload (`… base64UrlSafeDecode decode "utf-8" contains "\"iss\":\"auth-hub\""`).
 * That accepted a token that merely *mentioned* the issuer anywhere, could not
 * express "the audience is exactly this one value", and — the real gap —
 * verified no signature at all. A token signed with the wrong key, or with
 * `alg: none`, passed every one of those assertions.
 *
 * Grounded in `internal/infrastructure/token/jwt.go` (claim set and signing
 * method) and `compose/compose.staging.yaml` (BACKEND_TOKEN_ISSUER,
 * BACKEND_TOKEN_AUDIENCE, BACKEND_TOKEN_TTL, and the
 * `alt_backend_token_secret` mount shared with alt-backend).
 */

async function backendTokenFromValidate(
	hub: APIRequestContext,
	cookieHeader: string,
): Promise<string> {
	const response = await hub.get("/validate", { headers: { Cookie: cookieHeader } });
	await expectStatus(response, 200);
	const token = response.headers()["x-alt-backend-token"];
	expect(token, "X-Alt-Backend-Token on a successful /validate").toBeTruthy();
	return token as string;
}

test.describe("backend token", () => {
	test("is signed HS256 and verifies against the shared secret @contract", async ({
		hub,
		session,
	}) => {
		// The assertion the Hurl suite could not make. `BACKEND_TOKEN_SECRET_FILE`
		// and alt-backend's own secret are the same compose secret
		// (`e2e/fixtures/staging-secrets/alt_backend_token_secret.txt`), so a
		// token that fails here is a token alt-backend would reject — the failure
		// mode where auth-hub keeps answering 200 and every downstream request
		// 401s.
		const token = await backendTokenFromValidate(hub, session.cookieHeader);
		const decoded = decodeJwt(token);

		// `alg` is asserted explicitly because "the signature verifies" is
		// vacuously true for `alg: none`, and a verifier that trusts the header's
		// algorithm is the classic JWT confusion attack. auth-hub must only ever
		// emit HS256 (jwt.NewWithClaims(jwt.SigningMethodHS256, …)).
		expect(decoded.header["alg"], "JOSE header alg").toBe("HS256");
		expect(decoded.header["typ"], "JOSE header typ").toBe("JWT");

		expect(
			verifyHS256(token, env.backendTokenSecret),
			"HMAC-SHA256 over the token's signing input did not match the shared secret",
		).toBe(true);
	});

	test("carries the issuer, audience and subject alt-backend expects @contract", async ({
		hub,
		session,
	}) => {
		const decoded = decodeJwt(await backendTokenFromValidate(hub, session.cookieHeader));

		expect(stringClaim(decoded, "iss"), "iss").toBe(env.jwtIssuer);
		// Exact list equality, not "contains". An extra audience is a token that
		// a second, unintended service would also accept — which is the thing
		// `aud` exists to prevent.
		expect(audiences(decoded), "aud").toEqual([env.jwtAudience]);
		expect(stringClaim(decoded, "sub"), "sub").toBe(session.userId);
	});

	test("carries the tenant, email, role and session claims @contract", async ({
		hub,
		session,
	}) => {
		// `backendClaims` in jwt.go. None of these four were asserted by
		// `12-validate-jwt-shape.hurl`, and alt-backend reads all of them.
		const decoded = decodeJwt(await backendTokenFromValidate(hub, session.cookieHeader));

		// Single-tenant: `IssueBackendToken` falls back to UserID when the
		// identity carries no explicit tenant. The fallback is deliberate and
		// documented, so asserting the resulting equality — rather than merely
		// "tenant_id is a UUID" — is what pins it.
		expect(stringClaim(decoded, "tenant_id"), "tenant_id").toBe(session.userId);
		expect(stringClaim(decoded, "email"), "email").toBe(session.email);

		// The staging identity schema (`e2e/fixtures/auth-hub/kratos/
		// identity.schema.json`) declares `additionalProperties: false` over
		// traits and defines no `role`, so Kratos can never return one and
		// `gateway/kratos.go` defaults to "user". Anything else on the wire means
		// the gateway's normalisation was bypassed — a privilege-escalation
		// shaped bug, not a cosmetic one.
		expect(stringClaim(decoded, "role"), "role").toBe("user");

		// `sid` is the session binding: it is what lets alt-backend tie an action
		// back to a specific Kratos session rather than only to a user.
		expect(stringClaim(decoded, "sid"), "sid").toBe(session.cookie);
	});

	test("expires exactly BACKEND_TOKEN_TTL after issue @contract", async ({ hub, session }) => {
		// compose.staging.yaml sets BACKEND_TOKEN_TTL=30m; `JWT_TTL_SECONDS` in
		// run.sh mirrors it, so a change to one without the other fails here
		// rather than silently widening the window a stolen token stays valid.
		//
		// Hurl could only assert `matches "\"exp\":[0-9]{10}"` — true of any
		// timestamp between 2001 and 2286, including one already in the past.
		const decoded = decodeJwt(await backendTokenFromValidate(hub, session.cookieHeader));

		const iat = numberClaim(decoded, "iat");
		const exp = numberClaim(decoded, "exp");
		expect(exp - iat, "exp - iat should be BACKEND_TOKEN_TTL").toBe(env.jwtTTLSeconds);

		// And it must be issued *now*, not replayed from a fixture. A 5-minute
		// band absorbs any clock skew between the test container and auth-hub;
		// both run on the same daemon, so anything wider would stop being an
		// assertion.
		const now = Math.floor(Date.now() / 1000);
		expect(Math.abs(now - iat), "iat should be within 5 minutes of now").toBeLessThan(300);
		expect(exp, "exp should be in the future").toBeGreaterThan(now);
	});

	test("the /session token is the same contract as the /validate token @contract", async ({
		hub,
		session,
	}) => {
		// `/session` mints its token through `usecase.GetSession` and `/validate`
		// through the handler's own `TokenIssuer` call. Two call sites, one
		// contract — and nothing asserted the second one at all: `05-session-happy.hurl`
		// only checked the header matched a JWS-shaped regex.
		//
		// The SPA reads its token from /session while nginx reads its token from
		// /validate. If these two ever diverge, exactly one of the two clients
		// breaks, which is the hardest kind of bug to attribute.
		const response = await hub.get("/session", { headers: { Cookie: session.cookieHeader } });
		await expectStatus(response, 200);
		const token = response.headers()["x-alt-backend-token"];
		expect(token, "X-Alt-Backend-Token on /session").toBeTruthy();

		const decoded = decodeJwt(token as string);
		expect(verifyHS256(token as string, env.backendTokenSecret), "signature").toBe(true);
		expect(stringClaim(decoded, "iss"), "iss").toBe(env.jwtIssuer);
		expect(audiences(decoded), "aud").toEqual([env.jwtAudience]);
		expect(stringClaim(decoded, "sub"), "sub").toBe(session.userId);
		expect(stringClaim(decoded, "tenant_id"), "tenant_id").toBe(session.userId);
		expect(numberClaim(decoded, "exp") - numberClaim(decoded, "iat"), "ttl").toBe(
			env.jwtTTLSeconds,
		);
	});

	test("a token minted for one session is not valid for another @contract", async ({
		hub,
		session,
		mintSession,
	}) => {
		// Two identities, two tokens, and the claims that must differ actually
		// differ. This is the assertion that would catch the worst plausible
		// caching bug in this service: `session_cache.go` is keyed on the cookie
		// value, so a key collision or a cache that keyed on something coarser
		// would hand user B a token asserting user A's subject — an authenticated
		// impersonation that every status-code assertion in this suite reports as
		// a clean 200.
		const other = await mintSession();

		const mine = decodeJwt(await backendTokenFromValidate(hub, session.cookieHeader));
		const theirs = decodeJwt(await backendTokenFromValidate(hub, other.cookieHeader));

		expect(stringClaim(mine, "sub")).not.toBe(stringClaim(theirs, "sub"));
		expect(stringClaim(theirs, "sub")).toBe(other.userId);
		expect(stringClaim(theirs, "email")).toBe(other.email);
		expect(stringClaim(theirs, "sid")).toBe(other.cookie);
	});
});

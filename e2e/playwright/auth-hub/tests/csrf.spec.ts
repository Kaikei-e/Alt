import { test, expect } from "../src/fixtures.js";
import { expectJson, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { INVALID_SESSION_VALUE } from "../src/env.js";
import {
	csrfMissingCookieErrorSchema,
	csrfResponseSchema,
	echoErrorSchema,
} from "../src/schemas.js";

/**
 * `POST /csrf` — the double-submit token mint. Ports `07` and `08`.
 *
 * `07-csrf-happy.hurl` asserted `matches "^[A-Za-z0-9_+/=.-]{16,}$"`, which
 * accepts any sixteen characters — including a token with no separator, no
 * timestamp and a truncated MAC. The real format is fully determined by
 * `internal/infrastructure/token/csrf.go`:
 *
 *     "<unix seconds>.<base64url-padded HMAC-SHA256(secret, sessionID|unix)>"
 *
 * and every part of it is load-bearing: the timestamp is what makes the token
 * expire (DefaultCSRFTTL = 1h), the session id inside the MAC is what binds it
 * to one session, and the 32-byte digest is what makes it unforgeable. This
 * file asserts all three.
 *
 * `/csrf` also has the one error envelope in the service that is not Echo's:
 * `csrf.go:34-40` writes `c.JSON(401, {"error": …})` directly for a missing
 * Cookie header, while every other 401 in auth-hub comes back as
 * `{"message": …}`. Both are asserted, because a spec that used the wrong
 * schema would be reporting on a handler it is not actually reaching.
 */

test.describe("csrf — authenticated", () => {
	test("POST /csrf mints a token @smoke", async ({ hub, session }) => {
		await expectJsonStatus(
			await hub.post("/csrf", { headers: { Cookie: session.cookieHeader } }),
			200,
			csrfResponseSchema,
		);
	});

	test("the token is a fresh timestamp and a full-width MAC @contract", async ({
		hub,
		session,
	}) => {
		const body = await expectJsonStatus(
			await hub.post("/csrf", { headers: { Cookie: session.cookieHeader } }),
			200,
			csrfResponseSchema,
		);

		const [rawTimestamp, rawMac] = body.data.csrf_token.split(".") as [string, string];

		// The timestamp is not decoration: `Validate` rejects a token older than
		// DefaultCSRFTTL, so a generator that emitted a constant — or a zero —
		// would mint tokens that are either immortal or born expired. Both pass a
		// character-class regex.
		const issuedAt = Number.parseInt(rawTimestamp, 10);
		const now = Math.floor(Date.now() / 1000);
		expect(Math.abs(now - issuedAt), "csrf token timestamp should be ~now").toBeLessThan(300);

		// 32 bytes is the whole SHA-256 digest. A generator that truncated to the
		// first 8 bytes still round-trips through `Validate` and still matches the
		// old Hurl regex, while having thrown away 192 bits of forgery resistance
		// — a silent downgrade that only a length assertion catches.
		expect(Buffer.from(rawMac, "base64url").length, "HMAC-SHA256 digest length").toBe(32);
	});

	test("tokens are bound to the session that asked for them @authz", async ({
		hub,
		session,
		mintSession,
	}) => {
		// The security property of a double-submit token: `sign()` MACs
		// `sessionID + "|" + ts`, so a token minted for session A must not be
		// producible for — or reusable by — session B. The Hurl suite had one
		// identity and could not express this at all.
		//
		// Two mints in the same wall-clock second share the timestamp segment, so
		// the MACs are the only thing that can differ. That makes this a direct
		// assertion about the HMAC input rather than about clock resolution: if
		// `Generate` ever stopped mixing the session id in, the two tokens would
		// come back byte-identical.
		const other = await mintSession();

		const [mine, theirs] = await Promise.all([
			hub.post("/csrf", { headers: { Cookie: session.cookieHeader } }),
			hub.post("/csrf", { headers: { Cookie: other.cookieHeader } }),
		]);

		const a = await expectJsonStatus(mine, 200, csrfResponseSchema);
		const b = await expectJsonStatus(theirs, 200, csrfResponseSchema);
		expect(a.data.csrf_token).not.toBe(b.data.csrf_token);

		const macA = a.data.csrf_token.split(".")[1];
		const macB = b.data.csrf_token.split(".")[1];
		expect(macA, "the MAC halves must differ even when the timestamps match").not.toBe(macB);
	});
});

test.describe("csrf — rejected", () => {
	test("/csrf with no Cookie header at all → 401 with the bare error envelope @authz", async ({ hub }) => {
		// `csrf.go` reads the *raw* Cookie header rather than `c.Cookie`, so a
		// completely absent header is the trigger here, and the response is the
		// handler's own `c.JSON` rather than Echo's error renderer. Two different
		// shapes for what looks like one situation; a client that only handles
		// `{"message": …}` sees an empty object.
		const response = await hub.post("/csrf");
		await expectStatus(response, 401);
		await expectJson(response, csrfMissingCookieErrorSchema);
	});

	test("/csrf with a Cookie header that has no session → 401 @authz", async ({ hub }) => {
		// The bypass probe for the raw-header check. `rawCookie` is non-empty, so
		// the short-circuit does *not* fire; `extractSessionID` returns "" and the
		// usecase hands the whole junk header to Kratos, which rejects it. If this
		// ever answered 200 it would mean the presence of any cookie was being
		// treated as authentication.
		//
		// The envelope differs from the test above — this one comes back through
		// `mapDomainError` → `echo.NewHTTPError`, hence `{"message": …}`. Asserting
		// the shape is how this test proves it took the deeper path rather than
		// the short-circuit.
		const response = await hub.post("/csrf", {
			headers: { Cookie: "ajs_anonymous_id=abc; locale=ja-JP" },
		});
		await expectStatus(response, 401);
		await expectJson(response, echoErrorSchema);
	});

	test("/csrf with an unknown session value → 401 @authz", async ({ hub }) => {
		// New: `08-csrf-unauth.hurl` only covered the missing header. `usecase.
		// GenerateCSRF` validates against Kratos *before* minting, so this is the
		// branch that proves a CSRF token cannot be obtained without a real
		// session — the whole point of the endpoint being authenticated.
		const response = await hub.post("/csrf", {
			headers: { Cookie: `ory_kratos_session=${INVALID_SESSION_VALUE}` },
		});
		await expectStatus(response, 401);
		await expectJson(response, echoErrorSchema);
	});

	test("a rejected /csrf returns no token field @authz", async ({ hub }) => {
		// Belt and braces on the negative: the failure worth fencing is a handler
		// that answers 401 *and* still includes a usable token in the body, which
		// a status-only assertion cannot see.
		const response = await hub.post("/csrf");
		await expectStatus(response, 401);
		expect(await response.text()).not.toContain("csrf_token");
	});

	test("GET /csrf is not routed @contract", async ({ hub }) => {
		// `e.POST("/csrf", ...)` — the only POST route in the service. A GET that
		// answered 200 would make the token mintable from a plain `<img>` tag.
		await expectStatus(await hub.get("/csrf"), 405);
	});
});

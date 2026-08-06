import { test, expect } from "../src/fixtures.js";
import { expectHeader, expectNoHeader, expectStatus } from "../../_shared/http.js";

/**
 * The hardened response header set — entirely new coverage.
 *
 * `middleware/SecurityHeaders()` is installed first in the chain
 * (`main.go:117`, ahead of tracing, logging and Recover) and sets seven
 * headers on every response. Nothing in the Hurl suite looked at any of them.
 *
 * They are exactly the kind of thing that survives a refactor by being
 * silently dropped: they have no functional effect a developer would notice,
 * `middleware/security_headers_test.go` asserts the middleware's own output
 * rather than the server's, and the one line that installs it is easy to lose
 * when reordering the chain. The interesting assertions here are therefore not
 * "the header exists on a 200" but "the header survives the error paths" —
 * `Recover()`, Echo's error renderer and the rate limiter all answer without
 * reaching a handler, and a middleware installed in the wrong position stops
 * covering precisely those.
 */

/** Exactly what `middleware/security_headers.go` sets, value for value. */
const EXPECTED_HEADERS: ReadonlyArray<readonly [string, string]> = [
	["Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload"],
	["X-Content-Type-Options", "nosniff"],
	["X-Frame-Options", "DENY"],
	["Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'"],
	["Referrer-Policy", "strict-origin-when-cross-origin"],
	["Permissions-Policy", "camera=(), microphone=(), geolocation=()"],
	["Cache-Control", "no-store, no-cache, must-revalidate, private"],
];

test.describe("security headers", () => {
	test("the full set is present on a 200 @contract", async ({ hub }) => {
		const response = await hub.get("/health");
		await expectStatus(response, 200);
		for (const [name, value] of EXPECTED_HEADERS) {
			expectHeader(response, name, value);
		}
	});

	test("the full set survives a 401 @contract", async ({ hub }) => {
		// A 401 from `/validate` is produced by Echo's error renderer, not by the
		// handler. `SecurityHeaders` runs before the handler and writes into the
		// response header map, so the headers persist — unless it is ever moved
		// below `Recover()` or below the error handler, at which point the
		// unauthenticated responses (the ones an attacker sees most of) stop
		// being hardened while the happy path still looks fine.
		const response = await hub.get("/validate");
		await expectStatus(response, 401);
		for (const [name, value] of EXPECTED_HEADERS) {
			expectHeader(response, name, value);
		}
	});

	test("the full set survives a 404 @contract", async ({ hub }) => {
		// Echo's `NotFoundHandler` never reaches a route, so route middleware
		// does not run for it — only global middleware does. This is the test
		// that distinguishes "installed with `e.Use`" from "installed per route".
		const response = await hub.get("/definitely-not-a-route");
		await expectStatus(response, 404);
		for (const [name, value] of EXPECTED_HEADERS) {
			expectHeader(response, name, value);
		}
	});

	test("an authorization decision is never cached @contract", async ({ hub, session }) => {
		// The most consequential of the seven for this particular service.
		// `/validate` is nginx's `auth_request` target and its response carries a
		// live bearer token in `X-Alt-Backend-Token`. `no-store` is what stops
		// nginx's proxy cache, a CDN, or a shared corporate proxy from replaying
		// one user's authorization decision — and their JWT — to the next
		// request that happens to look the same.
		const response = await hub.get("/validate", {
			headers: { Cookie: session.cookieHeader },
		});
		await expectStatus(response, 200);
		expectHeader(response, "Cache-Control", "no-store, no-cache, must-revalidate, private");
	});

	test("responses do not advertise the implementation @contract", async ({ hub }) => {
		// Echo sets neither by default, and `main.go` sets `HideBanner`/`HidePort`
		// for the same reason. Asserting the absence is what keeps a future
		// `middleware.AddTrailingSlash`-style convenience — or a reverse proxy
		// misconfiguration inside the container — from quietly reintroducing one.
		const response = await hub.get("/health");
		expectNoHeader(response, "X-Powered-By");
		expectNoHeader(response, "Server");
	});

	test("the JSON responses declare a JSON content type @contract", async ({ hub, session }) => {
		// `X-Content-Type-Options: nosniff` above is only meaningful if the
		// declared type is correct in the first place: with nosniff set, a body
		// served as text/plain will not be parsed as JSON by a browser fetch, so
		// the two assertions are halves of one contract.
		for (const response of [
			await hub.get("/health"),
			await hub.get("/session", { headers: { Cookie: session.cookieHeader } }),
		]) {
			expect(
				response.headers()["content-type"],
				`Content-Type of ${response.url()}`,
			).toContain("application/json");
		}
	});
});

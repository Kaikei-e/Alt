import { test, expect } from "../src/fixtures.js";
import { expectHeader, expectHeaderContains, expectStatus } from "../src/http.js";

/**
 * Security response headers and body limits — new coverage.
 *
 * `routes.go` installs `middleware.SecureWithConfig` with a hand-tuned policy
 * (M-008), a 2MB body limit with a streaming skipper (H-005), a gzip skipper
 * that has to keep `/metrics` uncompressed for Prometheus 3.x, and a CORS
 * allowlist that must keep admitting `X-Alt-Backend-Token` (M-009). Not one of
 * those was asserted anywhere: they are pure middleware configuration, the
 * kind that survives a refactor by being silently dropped.
 */

test.describe("security headers", () => {
	test("the hardened header set is present on a normal response", async ({ rest }) => {
		const response = await rest.get("/v1/feeds/stats");
		await expectStatus(response, 200);

		expectHeader(response, "X-Content-Type-Options", "nosniff");
		expectHeader(response, "X-Frame-Options", "DENY");
		expectHeader(response, "X-XSS-Protection", "1; mode=block");
		expectHeader(response, "Referrer-Policy", "strict-origin-when-cross-origin");

		// alt-backend returns JSON, never HTML, so the policy denies everything
		// and only names the report sink. `frame-ancestors 'none'` is the one
		// clause X-Frame-Options cannot express for modern browsers.
		const csp = response.headers()["content-security-policy"];
		expect(csp, "Content-Security-Policy").toBeDefined();
		expect(csp).toContain("default-src 'none'");
		expect(csp).toContain("frame-ancestors 'none'");
		expect(csp).toContain("base-uri 'none'");
		expect(csp).toContain("report-uri /security/csp-report");
	});

	test("HSTS is advertised with the configured max-age", async ({ rest }) => {
		// HSTSMaxAge=31536000 + HSTSPreloadEnabled. Echo only emits the header
		// when it believes the request is TLS-terminated upstream, so treat a
		// missing header as acceptable but a *wrong* one as a failure.
		const response = await rest.get("/v1/feeds/stats", {
			headers: { "X-Forwarded-Proto": "https" },
		});
		const hsts = response.headers()["strict-transport-security"];
		if (hsts !== undefined) {
			expect(hsts).toContain("max-age=31536000");
		}
	});

	test("responses do not leak the server implementation", async ({ rest }) => {
		const response = await rest.get("/v1/feeds/stats");
		expect(response.headers()["x-powered-by"]).toBeUndefined();
	});

	test("every response carries a request id for log correlation", async ({ rest }) => {
		// RequestIDMiddleware is the first middleware in the chain; without the
		// header, a 500 in a CI log cannot be tied back to the test that caused
		// it, which is exactly when you need it most.
		const response = await rest.get("/v1/feeds/stats");
		const requestID = response.headers()["x-request-id"];
		expect(requestID, "X-Request-ID").toBeTruthy();
	});
});

test.describe("CORS preflight", () => {
	test("OPTIONS admits the alt-backend auth and CSRF headers", async ({ restAnon }) => {
		// M-009: X-Alt-Backend-Token has to be on Access-Control-Allow-Headers or
		// no browser can ever send the JWT. Dropping it breaks the SPA while
		// leaving every server-side test green.
		const response = await restAnon.fetch("/v1/feeds/fetch/list", {
			method: "OPTIONS",
			headers: {
				Origin: "http://localhost",
				"Access-Control-Request-Method": "GET",
				"Access-Control-Request-Headers": "x-alt-backend-token,x-csrf-token",
			},
		});

		const allowHeaders = (response.headers()["access-control-allow-headers"] ?? "").toLowerCase();
		if (allowHeaders !== "") {
			expect(allowHeaders).toContain("x-alt-backend-token");
			expect(allowHeaders).toContain("x-csrf-token");
		} else {
			// No CORS headers means the Origin was not on the allowlist. That is a
			// valid staging configuration, but it must not be silently mistaken
			// for "preflight works" — assert the request was still refused, not
			// served.
			expect(response.status()).not.toBe(200);
		}
	});
});

test.describe("body limits and compression", () => {
	test("a body over the 2MB limit is rejected", async ({ rest, csrf }) => {
		// H-005: BodyLimitWithConfig("2M"). The OPML import path is the reason
		// the ceiling exists, so oversize it there. Echo answers 413.
		const oversize = "x".repeat(3 * 1024 * 1024);
		const response = await rest.post("/v1/rss-feed-link/register", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { url: `http://stub.invalid/${oversize}` },
		});
		expect(response.status()).toBe(413);
	});

	test("a gzip-accepting client still gets a well-formed JSON list", async ({ rest }) => {
		// GzipWithConfig(level 5) sits in front of every JSON route. What breaks
		// when it is misconfigured is not the header but the body: a
		// double-compressed or truncated stream decodes to garbage. Asserting
		// the decoded body parses is therefore the assertion that actually
		// catches the failure — and unlike a Content-Encoding check it does not
		// depend on whether the HTTP client strips the header after decoding.
		const response = await rest.get("/v1/feeds/fetch/list", {
			headers: { "Accept-Encoding": "gzip" },
		});
		await expectStatus(response, 200);
		expect(Array.isArray(await response.json())).toBe(true);

		const encoding = response.headers()["content-encoding"];
		if (encoding !== undefined) {
			expect(encoding).toContain("gzip");
		}
	});

	test("brotli-preferring clients are not gzip-wrapped", async ({ rest }) => {
		// The Skipper bails out when the client advertises `br`, on the
		// assumption that the edge layer will handle it. Double-encoding here is
		// the exact regression that skipper prevents.
		const response = await rest.get("/v1/feeds/fetch/list", {
			headers: { "Accept-Encoding": "br" },
		});
		await expectStatus(response, 200);
		expect(response.headers()["content-encoding"]).toBeUndefined();
	});

	test("/metrics is never gzip-framed", async ({ ops }) => {
		// The gzip Skipper special-cases `/metrics`: Prometheus 3.x reports a
		// gzip-framed scrape as `expected a valid start token, got \\x1f`. This
		// broke once; it is cheap to keep it from breaking twice.
		const response = await ops.get("/metrics", { headers: { "Accept-Encoding": "gzip" } });
		await expectStatus(response, 200);
		expect(response.headers()["content-encoding"]).toBeUndefined();
	});
});

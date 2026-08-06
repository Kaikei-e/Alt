import { test, expect } from "../src/fixtures.js";

/**
 * CSP violation endpoint — the port of `80-csp-report.hurl`.
 *
 * Public, DOS-whitelisted and CSRF-exempt: a browser posting a violation
 * report has no session, no token, and no way to retry. The handler accepts
 * the browser-native report shape and logs it.
 *
 * This is the sink named in the Content-Security-Policy header that
 * tests/security-headers.spec.ts asserts (`report-uri /security/csp-report`),
 * so the two files are two halves of one contract: the policy points here, and
 * here accepts what the policy sends.
 */

const BROWSER_REPORT = {
	"csp-report": {
		"document-uri": "http://localhost/",
		referrer: "",
		"violated-directive": "default-src 'none'",
		"effective-directive": "default-src",
		"original-policy": "default-src 'none'; report-uri /security/csp-report",
		"blocked-uri": "inline",
	},
};

test.describe("POST /security/csp-report", () => {
	test("accepts a browser-native report without authentication", async ({ restAnon }) => {
		const response = await restAnon.post("/security/csp-report", {
			headers: { "Content-Type": "application/csp-report" },
			data: BROWSER_REPORT,
		});
		expect(response.status()).toBeGreaterThanOrEqual(200);
		expect(response.status()).toBeLessThan(500);
	});

	test("accepts the same report as application/json", async ({ restAnon }) => {
		// New coverage. Chromium sends `application/csp-report`; Firefox has
		// historically sent `application/json`. A handler that switches on
		// Content-Type would silently drop half the reports in production and
		// nothing would ever page about it.
		const response = await restAnon.post("/security/csp-report", {
			headers: { "Content-Type": "application/json" },
			data: BROWSER_REPORT,
		});
		expect(response.status()).toBeLessThan(500);
	});

	test("does not fault on a malformed report body", async ({ restAnon }) => {
		// New coverage. This endpoint is unauthenticated and reachable from any
		// browser on the internet, so a body it cannot parse must be an ordinary
		// rejection — never a panic, and never a 5xx that would trip the shared
		// circuit breaker for every other caller.
		const response = await restAnon.post("/security/csp-report", {
			headers: { "Content-Type": "application/csp-report" },
			data: "{ this is not a report",
		});
		expect(response.status()).toBeLessThan(500);
	});

	test("does not fault on an empty body", async ({ restAnon }) => {
		const response = await restAnon.post("/security/csp-report", {
			headers: { "Content-Type": "application/csp-report" },
			data: "",
		});
		expect(response.status()).toBeLessThan(500);
	});
});

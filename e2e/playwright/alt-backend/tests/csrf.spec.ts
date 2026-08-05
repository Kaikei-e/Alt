import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../src/http.js";
import { stubURL } from "../src/fixtures.js";
import { csrfTokenSchema } from "../src/schemas.js";

/**
 * CSRF middleware — new coverage.
 *
 * The Hurl suite captured a token in every mutating scenario and never once
 * asked what happens without one, so `CSRFMiddleware` was exercised only on
 * its success path. Its two rejection branches and its exemption list were
 * completely untested; an accidental `return next(c)` at the top would have
 * shipped green through all 25 scenarios.
 *
 * The middleware answers 403 with `{"message": {"error": ..., "message": ...}}`
 * — echo.NewHTTPError wraps the map it is handed — so these assert the status
 * and the discriminating `error` string, not a top-level envelope.
 */

async function csrfErrorCode(bodyText: string): Promise<string | undefined> {
	const parsed: unknown = JSON.parse(bodyText);
	if (typeof parsed !== "object" || parsed === null) return undefined;
	const message = (parsed as Record<string, unknown>)["message"];
	const carrier = typeof message === "object" && message !== null ? message : parsed;
	const code = (carrier as Record<string, unknown>)["error"];
	return typeof code === "string" ? code : undefined;
}

test.describe("CSRF middleware", () => {
	test("POST without X-CSRF-Token → 403 csrf_token_missing", async ({ rest }) => {
		const response = await rest.post("/v1/rss-feed-link/register", {
			headers: { "Content-Type": "application/json" },
			data: { url: stubURL("feed-csrf-missing.xml") },
		});
		await expectStatus(response, 403);
		expect(await csrfErrorCode(await response.text())).toBe("csrf_token_missing");
	});

	test("POST with an unminted token → 403 csrf_token_invalid", async ({ rest }) => {
		// The token is validated against the in-memory store, not merely checked
		// for presence. A middleware that only tested `!= ""` would pass the
		// case above and fail here — which is the whole point of having both.
		const response = await rest.post("/v1/rss-feed-link/register", {
			headers: {
				"Content-Type": "application/json",
				"X-CSRF-Token": "this-token-was-never-minted-by-the-server",
			},
			data: { url: stubURL("feed-csrf-invalid.xml") },
		});
		await expectStatus(response, 403);
		expect(await csrfErrorCode(await response.text())).toBe("csrf_token_invalid");
	});

	test("DELETE without a token is protected too", async ({ rest }) => {
		// isCSRFProtectedEndpoint defaults to protecting every unexempted
		// state-changing method, not just POST. A regression that narrowed the
		// method list to POST would leave DELETE wide open.
		const response = await rest.delete("/v1/rss-feed-link/00000000-0000-0000-0000-000000000000");
		await expectStatus(response, 403);
	});

	test("PATCH without a token is protected too", async ({ rest }) => {
		const response = await rest.patch(
			"/v1/admin/scraping-domains/00000000-0000-0000-0000-000000000000",
			{ headers: { "Content-Type": "application/json" }, data: { force_respect_robots: true } },
		);
		await expectStatus(response, 403);
	});

	test("GET is never CSRF-protected", async ({ rest }) => {
		await expectStatus(await rest.get("/v1/feeds/stats"), 200);
	});

	test("POST /security/csp-report is exempt", async ({ restAnon }) => {
		// Browsers post CSP reports without ever having seen a token. If this
		// endpoint became protected, every violation report would 403 and the
		// reporting channel would go silent without anyone noticing.
		const response = await restAnon.post("/security/csp-report", {
			headers: { "Content-Type": "application/csp-report" },
			data: { "csp-report": { "document-uri": "http://localhost/", "blocked-uri": "inline" } },
		});
		expect(response.status()).not.toBe(403);
	});

	test("a minted token is reusable — it is not consumed on first use", async ({ rest }) => {
		// csrf_token_gateway.ValidateToken only deletes the entry when it has
		// *expired*; a valid token survives validation. The whole suite's
		// worker-scoped token fixture depends on that, so assert it rather than
		// assume it.
		const minted = await expectJsonStatus(
			await rest.get("/v1/csrf-token"),
			200,
			csrfTokenSchema,
		);

		for (const attempt of [1, 2]) {
			const response = await rest.post("/v1/feeds/tags", {
				headers: { "Content-Type": "application/json", "X-CSRF-Token": minted.csrf_token },
				data: { feed_url: stubURL("feed-csrf-reuse.xml") },
			});
			expect(response.status(), `attempt ${attempt}`).not.toBe(403);
		}
	});
});

import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../src/http.js";
import { ZERO_UUID } from "../src/env.js";
import { plainErrorSchema, secureErrorSchema } from "../src/schemas.js";

/**
 * Admin scraping-domain policies — the port of
 * `71-admin-scraping-domains.hurl`, plus the `refresh-robots` route it never
 * touched.
 *
 * These endpoints control scraping consent policy (robots.txt compliance, ML
 * training opt-in), which is why the whole group sits behind
 * RequireAuth + RequireAdmin.
 */

test.describe("GET /v1/admin/scraping-domains", () => {
	test("lists domains", async ({ rest }) => {
		const response = await rest.get("/v1/admin/scraping-domains");
		await expectStatus(response, 200);
		expect(Array.isArray(await response.json())).toBe(true);
	});

	test("clamps an out-of-range limit rather than rejecting it", async ({ rest }) => {
		// New coverage: `if limit <= 0 || limit > 100 { limit = 20 }`. Unlike the
		// feed cursor endpoints — where an oversized limit is a 400 from
		// ValidationMiddleware — this handler silently substitutes a default.
		// Two conventions in one codebase; both are now pinned, so a future
		// unification is a visible decision.
		const response = await rest.get("/v1/admin/scraping-domains?limit=100000");
		await expectStatus(response, 200);
		expect((await response.json()).length).toBeLessThanOrEqual(100);
	});
});

test.describe("GET /v1/admin/scraping-domains/:id", () => {
	test("an unknown id is a deterministic 404", async ({ rest }) => {
		// GetByID returns (nil, nil) on no matching row — the driver turns
		// pgx.ErrNoRows into (nil, nil) rather than propagating an error — and
		// the handler maps a nil domain straight to 404. Deterministic, not
		// "accept both".
		const body = await expectJsonStatus(
			await rest.get(`/v1/admin/scraping-domains/${ZERO_UUID}`),
			404,
			plainErrorSchema,
		);
		expect(body.error).toBe("scraping domain not found");
	});

	test("a malformed id is a validation error", async ({ rest }) => {
		// New coverage. `uuid.Parse` failure routes to HandleValidationError,
		// which is a different envelope from the 404 above — worth pinning so a
		// refactor cannot collapse "you sent nonsense" into "not found".
		const response = await rest.get("/v1/admin/scraping-domains/not-a-uuid");
		await expectStatus(response, 400);
		expect(await response.text()).toContain("VALIDATION_ERROR");
	});
});

test.describe("PATCH /v1/admin/scraping-domains/:id", () => {
	test("KNOWN BUG: an unknown id answers 500, not 404", async ({ rest, csrf }) => {
		// Recorded, not fixed — production code is out of scope for this suite.
		//
		// UpdatePolicy's driver DOES check RowsAffected == 0 and returns an error
		// for a nonexistent id (scraping_domain_driver.go:339-342), unlike
		// GetByID's (nil, nil) pattern above — but that error is a bare
		// `errors.New`, not an AppContextError/AppError, so HandleError falls
		// through to its generic UNKNOWN_ERROR branch and answers 500 instead of
		// 404 (app_context_error.go:42-61 has no NOT_FOUND case to hit even if
		// it were typed). GET and PATCH on the same nonexistent id therefore
		// disagree on status code.
		//
		// This pins the real, current behaviour so a regression is still caught.
		// The fix is to return a typed NOT_FOUND from the driver; when that
		// lands, this test fails and should become a 404 assertion.
		const body = await expectJsonStatus(
			await rest.patch(`/v1/admin/scraping-domains/${ZERO_UUID}`, {
				headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
				data: { force_respect_robots: true },
			}),
			500,
			secureErrorSchema,
		);
		expect(body.error.code).toBe("UNKNOWN_ERROR");
	});

	test("a malformed id is a validation error", async ({ rest, csrf }) => {
		const response = await rest.patch("/v1/admin/scraping-domains/not-a-uuid", {
			headers: { "Content-Type": "application/json", "X-CSRF-Token": csrf },
			data: { force_respect_robots: true },
		});
		await expectStatus(response, 400);
	});
});

/**
 * `POST /:id/refresh-robots` — entirely new coverage.
 *
 * The route has existed with no E2E assertion of any kind, which means neither
 * its registration nor its admin gate was ever exercised end to end. It
 * re-fetches a domain's robots.txt, so it is both a mutating operation and an
 * outbound one: exactly the shape that should not be reachable without the
 * admin role.
 */
test.describe("POST /v1/admin/scraping-domains/:id/refresh-robots", () => {
	test("is registered and reached by an admin token", async ({ rest, csrf }) => {
		// The id does not exist, so the interesting assertion is that the answer
		// comes from the handler at all: 404 or 500 both mean "route resolved,
		// usecase ran". A 404 with an Echo router body would mean the route was
		// never registered — indistinguishable by status alone, which is why the
		// negative below (401 without a token) carries the other half of the
		// proof.
		const response = await rest.post(
			`/v1/admin/scraping-domains/${ZERO_UUID}/refresh-robots`,
			{ headers: { "X-CSRF-Token": csrf } },
		);
		expect(response.status()).toBeGreaterThanOrEqual(400);
		expect(response.status()).toBeLessThan(600);
		expect(response.status()).not.toBe(401);
		expect(response.status()).not.toBe(403);
	});

	test("a malformed id is a validation error", async ({ rest, csrf }) => {
		// This one *is* deterministic: uuid.Parse fails before any usecase call,
		// so a 400 here proves the handler is mounted and running.
		const response = await rest.post(
			"/v1/admin/scraping-domains/not-a-uuid/refresh-robots",
			{ headers: { "X-CSRF-Token": csrf } },
		);
		await expectStatus(response, 400);
		expect(await response.text()).toContain("VALIDATION_ERROR");
	});

	test("is refused without a token", async ({ restAnon, csrf }) => {
		const response = await restAnon.post(
			`/v1/admin/scraping-domains/${ZERO_UUID}/refresh-robots`,
			{ headers: { "X-CSRF-Token": csrf } },
		);
		await expectStatus(response, 401);
	});

	test("is refused without a CSRF token", async ({ rest }) => {
		await expectStatus(
			await rest.post(`/v1/admin/scraping-domains/${ZERO_UUID}/refresh-robots`),
			403,
		);
	});
});

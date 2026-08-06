import { test, expect } from "../src/fixtures.js";
import { expectStatus, expectStatusIn } from "../../_shared/http.js";
import { WRONG_SIGNATURE_JWT } from "../src/env.js";

/**
 * Admin dashboard — the port of `70-admin-dashboard.hurl`.
 *
 * `/v1/dashboard/*` chains RequireAuth + RequireAdmin; the pre-minted staging
 * JWT carries role=admin. The first four endpoints read local state and must
 * answer 200. `recap_jobs` is different — it fans out to recap-worker — so it
 * is asserted separately.
 */

const LOCAL_DASHBOARD_ROUTES = [
	"/v1/dashboard/metrics",
	"/v1/dashboard/overview",
	"/v1/dashboard/logs?limit=10",
	"/v1/dashboard/jobs",
] as const;

test.describe("admin dashboard reads", () => {
	for (const path of LOCAL_DASHBOARD_ROUTES) {
		test(`GET ${path} answers 200 with JSON`, async ({ rest }) => {
			const response = await rest.get(path);
			await expectStatus(response, 200);
			// The Hurl file asserted `$ exists` on the first and nothing at all on
			// the other three, so a 200 carrying an empty body would have passed.
			const text = await response.text();
			expect(() => JSON.parse(text), `body was not JSON: ${text.slice(0, 300)}`).not.toThrow();
		});
	}

	test("GET /v1/dashboard/recap_jobs tolerates the stub's missing route", async ({ rest }) => {
		// Unlike the four above, recap_jobs fans out to recap-worker, and the
		// deps-stub has no route for it — the request lands on its catch-all,
		// which answers 200 with a `{"status":"stub-noop"}` object where a job
		// array is expected. Decoding fails and HandleError turns that into 500
		// UNKNOWN_ERROR (the same stub-path drift as morning-letter).
		//
		// 4xx stays excluded — the auth/admin chain must still admit this JWT —
		// and 200 is accepted for when the stub grows the route.
		await expectStatusIn(await rest.get("/v1/dashboard/recap_jobs"), [200, 500]);
	});
});

test.describe("admin authorisation", () => {
	for (const path of LOCAL_DASHBOARD_ROUTES) {
		test(`GET ${path} is refused without a token`, async ({ restAnon }) => {
			await expectStatus(await restAnon.get(path), 401);
		});
	}

	test("a forged admin token is refused", async ({ restAnon }) => {
		// New coverage. RequireAdmin reads the role claim, so the interesting
		// question is not "does it read the claim" but "does anything verify the
		// signature before it does". A token whose payload says admin but whose
		// HMAC is wrong must never reach the role check.
		const response = await restAnon.get("/v1/dashboard/metrics", {
			headers: { "X-Alt-Backend-Token": WRONG_SIGNATURE_JWT },
		});
		await expectStatus(response, 401);
	});

	test("the dashboard group rejects unknown sub-paths without leaking auth state", async ({
		restAnon,
	}) => {
		// New coverage: an unregistered path under an authenticated group must
		// answer 401 (middleware first), not 404 — otherwise the route table is
		// enumerable by an unauthenticated caller.
		await expectStatus(await restAnon.get("/v1/dashboard/does-not-exist"), 401);
	});
});

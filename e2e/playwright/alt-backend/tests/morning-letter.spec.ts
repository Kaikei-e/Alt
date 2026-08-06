import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { secureErrorSchema } from "../src/schemas.js";

/**
 * Morning letter — the port of `40-morning-letter.hurl`.
 *
 * KNOWN BUG (recorded, not fixed — the deps-stub lives under e2e/stubs/, out
 * of scope for this suite):
 *
 * `GetOvernightUpdates`'s repository (morning_gateway.go:48-52) calls
 * `GET {RECAP_WORKER_URL}/v1/morning/updates?since=…` and decodes the body as
 * a JSON *array* (`[]MorningArticleGroupResponse`). The deps-stub only
 * implements `GET /morning-letter/{user_id}` — a *document*, used by the
 * unrelated MorningLetterGateway for `/v1/morning/letters/*` — and has no
 * route for `/v1/morning/updates`, so the request falls through to the stub's
 * catch-all, which answers 200 with a JSON *object*
 * (`{"status":"stub-noop",…}`). Decoding an object into a slice fails,
 * morning_gateway.go:82-85 turns that into an error, and handleMorningUpdates
 * maps it to a generic 500.
 *
 * This is exactly the "stub drift" the deps-stub's own module docstring calls
 * out. The assertion pins the real, current behaviour so further regressions
 * are still caught — and so the day someone teaches the stub that route, this
 * test fails and gets tightened rather than quietly widening.
 */

test.describe("GET /v1/morning-letter/updates", () => {
	test("stub drift surfaces as a typed UNKNOWN_ERROR, not a crash", async ({ rest }) => {
		const body = await expectJsonStatus(
			await rest.get("/v1/morning-letter/updates"),
			500,
			secureErrorSchema,
		);
		expect(body.error.code).toBe("UNKNOWN_ERROR");
	});

	test("the failure is a typed envelope, not a leaked internal message", async ({ rest }) => {
		// New coverage. `SecureHTTPResponse` exists so a 500 cannot echo the
		// gateway's decode error — which would name the upstream host and its
		// response shape — back to the caller. The 500 above is expected today;
		// what must never happen is that it starts carrying the guts of the
		// failure with it.
		const body = await expectJsonStatus(
			await rest.get("/v1/morning-letter/updates"),
			500,
			secureErrorSchema,
		);
		expect(body.error.message).not.toContain("recap-worker");
		expect(body.error.message).not.toContain("json:");
		expect(body.error.message).not.toContain("stub-noop");
	});

	test("is behind authentication", async ({ restAnon }) => {
		const response = await restAnon.get("/v1/morning-letter/updates");
		expect(response.status()).toBe(401);
	});
});

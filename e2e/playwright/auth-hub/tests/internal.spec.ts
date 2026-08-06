import { test, expect } from "../src/fixtures.js";
import {
	expectJson,
	expectJsonStatus,
	expectStatus,
	expectStatusIn,
} from "../../_shared/http.js";
import { env } from "../src/env.js";
import { echoErrorSchema, systemUserSchema } from "../src/schemas.js";

/**
 * `/internal/system-user` — the service-to-service surface. Ports `09`, `10`
 * and `11`.
 *
 * This route has no per-user authorization behind it at all: a shared secret
 * in `X-Internal-Auth` is the entire control (`middleware/internal_auth.go`,
 * wired through `main.go`'s `wireInternalAuth`, which panics rather than
 * degrade to an unauthenticated no-op — CLAUDE.md rule 8). So the negatives
 * here are not edge cases, they are the security boundary, and the distinction
 * the middleware draws between *missing* (401) and *wrong* (403) is a contract
 * that operators read in logs.
 *
 * One assertion is deliberately different from the Hurl original.
 * `09-internal-system-user-happy.hurl` asserted `$.user_id == "{{user_id}}"`
 * because the whole run had exactly one Kratos identity, so "the first
 * identity" and "the identity we seeded" were necessarily the same row. This
 * suite seeds one identity per worker plus more inside individual tests, which
 * makes that equality an accident of ordering rather than a property of the
 * handler — the handler asks Kratos for `/admin/identities?page_size=1`
 * (`gateway/kratos.go`) and returns whichever row comes back. The replacement
 * asserts the stronger, order-independent fact: the id auth-hub reports is a
 * real, resolvable Kratos identity. A handler returning a stale cache entry, a
 * config default or a truncated string fails that; none of them would have
 * failed a UUID-shaped regex.
 */

/** A wrong secret of the same length as the real one — the timing-safe case. */
function lengthMatchedWrongSecret(): string {
	return "x".repeat(env.backendTokenSecret.length);
}

test.describe("internal system-user", () => {
	test("GET /internal/system-user returns a resolvable identity @contract", async ({
		hub,
		kratosAdmin,
	}) => {
		const body = await expectJsonStatus(
			await hub.get("/internal/system-user", {
				headers: { "X-Internal-Auth": env.backendTokenSecret },
			}),
			200,
			// `strict()`: this is a service-to-service response guarded only by a
			// shared secret, so any additional key is identity data crossing a
			// trust boundary that has no per-user check behind it.
			systemUserSchema,
		);

		const identity = await kratosAdmin.get(`/admin/identities/${body.user_id}`);
		await expectStatus(identity, 200);
		expect((await identity.json()) as { id?: unknown }).toMatchObject({ id: body.user_id });
	});

	test("the answer is stable across calls @contract", async ({ hub }) => {
		// `GetFirstIdentityID` is uncached and re-queries Kratos every time, so
		// two calls in quick succession must agree. They would not if the handler
		// were sensitive to Kratos's pagination ordering — which is exactly the
		// property alt-backend's harvester relies on when it attributes system-
		// initiated writes to a stable user.
		//
		// Two calls, not more: the /internal limiter is burst 3 (main.go:174) and
		// runs before the auth middleware, so this test's private bucket has room
		// for exactly three.
		const headers = { "X-Internal-Auth": env.backendTokenSecret };
		const first = await expectJsonStatus(
			await hub.get("/internal/system-user", { headers }),
			200,
			systemUserSchema,
		);
		const second = await expectJsonStatus(
			await hub.get("/internal/system-user", { headers }),
			200,
			systemUserSchema,
		);
		expect(second.user_id).toBe(first.user_id);
	});
});

test.describe("internal auth boundary", () => {
	test("no X-Internal-Auth header → 401 @authz", async ({ hub }) => {
		// internal_auth.go:19-21 — `len(provided) == 0` short-circuits before the
		// comparison. 401 rather than 403 is the deliberate signal "you did not
		// present a credential", which is what makes a misconfigured caller
		// distinguishable from a hostile one in the access log.
		const response = await hub.get("/internal/system-user");
		await expectStatus(response, 401);
		await expectJson(response, echoErrorSchema);
	});

	test("an empty X-Internal-Auth header → 401, not 403 @authz", async ({ hub }) => {
		// New. `""` is a header that is *present* but carries nothing, and it is
		// what a caller with an unset environment variable actually sends —
		// CLAUDE.md rule 9's failure mode arriving over the wire. The middleware
		// measures the value, not the header, so this must land on the same 401
		// branch as a missing header rather than on the 403 "invalid" branch.
		const response = await hub.get("/internal/system-user", {
			headers: { "X-Internal-Auth": "" },
		});
		await expectStatus(response, 401);
	});

	test("a wrong secret of the same length → 403 @authz", async ({ hub }) => {
		// internal_auth.go:22-24. `subtle.ConstantTimeCompare` returns 0, and the
		// middleware distinguishes invalid (403) from missing (401). Matching the
		// real secret's length is what makes this exercise the comparison itself
		// rather than the length check inside it. Timing-safety proper is asserted
		// in auth-hub/middleware/internal_auth_test.go; this is the shape.
		const response = await hub.get("/internal/system-user", {
			headers: { "X-Internal-Auth": lengthMatchedWrongSecret() },
		});
		await expectStatus(response, 403);
		await expectJson(response, echoErrorSchema);
	});

	test("a wrong secret of a different length → 403 @authz", async ({ hub }) => {
		// New. `ConstantTimeCompare` returns 0 for unequal lengths without
		// comparing anything, so this reaches the same branch by a different
		// route. It is worth its own test because the natural "optimisation" —
		// an early `len(provided) != len(secret)` return — is where a
		// distinguishable status or a different code path tends to get
		// introduced.
		const response = await hub.get("/internal/system-user", {
			headers: { "X-Internal-Auth": "short" },
		});
		await expectStatus(response, 403);
	});

	test("a rejected call leaks no identity @authz", async ({ hub }) => {
		// The body of a 403 must not contain a user id. `mapDomainError` is never
		// reached on this path, but a future handler that logged-and-returned the
		// system user before checking auth would still answer 403 — and would
		// still hand out the very thing the endpoint exists to protect.
		const response = await hub.get("/internal/system-user", {
			headers: { "X-Internal-Auth": lengthMatchedWrongSecret() },
		});
		await expectStatus(response, 403);
		expect(await response.text()).not.toContain("user_id");
	});

	test("POST /internal/system-user is not routed @contract", async ({ hub }) => {
		// `internalGroup.GET("/system-user", ...)` — GET is the only verb.
		//
		// A band, and both members are correct answers to the same question.
		// Echo's `Group.Use` additionally registers a catch-all `/internal/*`
		// bound to `NotFoundHandler` (so that group middleware still runs for
		// unmatched paths), which means a verb mismatch on an exact child route
		// can resolve either to that route's 405 or to the catch-all's 404
		// depending on how the router backtracks. Both say "this verb is not
		// served here". What is outside the band is what matters: a 2xx would
		// mean POST reached the handler, and — since this call carries the
		// correct shared secret — a 401 or 403 would mean the group middleware
		// ran in an order this test does not model.
		const response = await hub.post("/internal/system-user", {
			headers: { "X-Internal-Auth": env.backendTokenSecret },
		});
		await expectStatusIn(response, [404, 405]);
	});

	test("an unregistered /internal path 404s even with the secret @contract", async ({ hub }) => {
		// The group is `/internal`, but only `/system-user` is mounted under it.
		// A valid secret must not make the group itself a wildcard: 404 says the
		// route table has no entry, which is the only status that distinguishes
		// "not here" from "here but refused". Presenting the *correct* secret is
		// what makes the assertion meaningful — a 404 from an unauthenticated
		// probe could just be a middleware ordering artefact.
		await expectStatus(
			await hub.get("/internal/identities", {
				headers: { "X-Internal-Auth": env.backendTokenSecret },
			}),
			404,
		);
	});
});

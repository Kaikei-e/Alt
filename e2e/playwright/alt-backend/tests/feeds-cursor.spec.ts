import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../src/http.js";
import {
	dataOnlyCursorPageSchema,
	fullCursorPageSchema,
	unreadCountSchema,
} from "../src/schemas.js";

/**
 * Cursor-paginated reads — the port of `21-feeds-fetch-cursor.hurl`.
 *
 * The Hurl file's opening claim was that "all three cursor endpoints share the
 * same response envelope: {data, has_more, next_cursor}". They do not, and the
 * first CI run of this suite is what established it. The file asserted
 * `jsonpath "$.data" isCollection` + `has_more isBoolean` on the *unread*
 * endpoint only, and `jsonpath "$" exists` — true of any JSON whatsoever — on
 * the other two, so the divergence was invisible.
 *
 * Two axes diverge, and each is pinned below rather than smoothed over:
 *
 * 1. **Envelope.** `/fetch/cursor` returns the typed
 *    `ArticlesWithCursorResponse` (`{data, has_more, next_cursor?}`).
 *    `/fetch/viewed/cursor` and `/fetch/favorites/cursor` build an untyped map
 *    with `data` and a conditional `next_cursor`, and no `has_more`.
 *
 * 2. **Limit handling.** ValidationMiddleware's `validateRoute` matches
 *    `strings.Contains(path, "/feeds/fetch/cursor")`, which
 *    `/v1/feeds/fetch/viewed/cursor` does not contain. So only the unread
 *    endpoint gets PaginationValidator's ceiling (reject above 1000); the
 *    other two fall through to their handler, which silently clamps anything
 *    over 100. Same-looking query, two different contracts.
 *
 * These are pins on current behaviour, not endorsements. Unifying the three is
 * a product/API decision; when it happens these tests fail and say so.
 */

const ALL_CURSOR_ENDPOINTS = [
	{ name: "unread", path: "/v1/feeds/fetch/cursor" },
	{ name: "viewed", path: "/v1/feeds/fetch/viewed/cursor" },
	{ name: "favorites", path: "/v1/feeds/fetch/favorites/cursor" },
] as const;

/** The two that share the untyped, `has_more`-less envelope. */
const DATA_ONLY_ENDPOINTS = ALL_CURSOR_ENDPOINTS.filter((e) => e.name !== "unread");

test.describe("envelope", () => {
	test("unread returns the typed {data, has_more, next_cursor?} envelope", async ({ rest }) => {
		await expectJsonStatus(
			await rest.get("/v1/feeds/fetch/cursor?limit=20"),
			200,
			fullCursorPageSchema,
		);
	});

	for (const endpoint of DATA_ONLY_ENDPOINTS) {
		test(`${endpoint.name} returns {data, next_cursor?} — no has_more`, async ({ rest }) => {
			await expectJsonStatus(
				await rest.get(`${endpoint.path}?limit=20`),
				200,
				dataOnlyCursorPageSchema,
			);
		});
	}
});

test.describe("input validation", () => {
	for (const endpoint of ALL_CURSOR_ENDPOINTS) {
		test(`${endpoint.name} rejects a non-RFC3339 cursor`, async ({ rest }) => {
			// The one rule all three do agree on, though they reach it by
			// different routes: the unread endpoint via PaginationValidator, the
			// other two via their own `time.Parse(time.RFC3339, ...)` check.
			await expectStatus(await rest.get(`${endpoint.path}?cursor=not-rfc3339`), 400);
		});

		test(`${endpoint.name} rejects limit=0`, async ({ rest }) => {
			await expectStatus(await rest.get(`${endpoint.path}?limit=0`), 400);
		});

		test(`${endpoint.name} rejects a non-numeric limit`, async ({ rest }) => {
			await expectStatus(await rest.get(`${endpoint.path}?limit=abc`), 400);
		});
	}

	test("unread rejects a limit above the validator ceiling", async ({ rest }) => {
		// PaginationValidator: "Limit too large (maximum 1000)". This is the
		// only one of the three that ValidationMiddleware sees.
		await expectStatus(await rest.get("/v1/feeds/fetch/cursor?limit=1001"), 400);
	});

	for (const endpoint of DATA_ONLY_ENDPOINTS) {
		test(`${endpoint.name} clamps a limit above 1000 instead of rejecting it`, async ({
			rest,
		}) => {
			// The mirror image of the assertion above, and the reason this suite
			// states both: ValidationMiddleware never matches this path, so the
			// handler's own `else if parsedLimit > 100 { limit = 100 }` branch is
			// the only rule in play and the request succeeds.
			const page = await expectJsonStatus(
				await rest.get(`${endpoint.path}?limit=1001`),
				200,
				dataOnlyCursorPageSchema,
			);
			expect(page.data?.length ?? 0).toBeLessThanOrEqual(100);
		});
	}
});

test.describe("pagination behaviour", () => {
	test("an oversized but legal limit is clamped on the unread endpoint", async ({ rest }) => {
		// 1000 passes the validator (its ceiling) and is then clamped to 100 by
		// the handler. Two layers, two different numbers — a refactor that
		// collapsed them into one would break exactly here.
		const page = await expectJsonStatus(
			await rest.get("/v1/feeds/fetch/cursor?limit=1000"),
			200,
			fullCursorPageSchema,
		);
		expect(page.data?.length ?? 0).toBeLessThanOrEqual(100);
	});

	test("following next_cursor advances the page", async ({ rest, seededFeeds }) => {
		// New coverage: the Hurl suite read page 1 and stopped, so a cursor that
		// was emitted but never honoured — the classic pagination bug that loops
		// forever on page 1 — was invisible. Three seeded feeds guarantee this
		// worker has rows to walk.
		expect(seededFeeds.length).toBe(3);

		const first = await expectJsonStatus(
			await rest.get("/v1/feeds/fetch/cursor?limit=1"),
			200,
			fullCursorPageSchema,
		);

		test.skip(!first.has_more, "single page of results — nothing to advance to");

		const cursor = first.next_cursor;
		expect(typeof cursor, "has_more implies a cursor").toBe("string");

		const second = await expectJsonStatus(
			await rest.get(`/v1/feeds/fetch/cursor?limit=1&cursor=${encodeURIComponent(cursor ?? "")}`),
			200,
			fullCursorPageSchema,
		);

		// The second page must not repeat the first. Comparing serialised items
		// keeps this independent of which fields the item shape carries.
		const firstItem = JSON.stringify(first.data?.[0] ?? null);
		const secondItem = JSON.stringify(second.data?.[0] ?? null);
		if (secondItem !== "null") {
			expect(secondItem, "next_cursor must advance past the first item").not.toBe(firstItem);
		}
	});
});

test.describe("GET /v1/feeds/count/unreads", () => {
	test("returns a non-negative integer count", async ({ rest }) => {
		await expectJsonStatus(await rest.get("/v1/feeds/count/unreads"), 200, unreadCountSchema);
	});
});

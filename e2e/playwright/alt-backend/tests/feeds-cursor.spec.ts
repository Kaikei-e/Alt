import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus, expectStatus } from "../src/http.js";
import { cursorPageSchema, unreadCountSchema } from "../src/schemas.js";

/**
 * Cursor-paginated reads — the port of `21-feeds-fetch-cursor.hurl`.
 *
 * All three cursor endpoints share the `{data, has_more, next_cursor}`
 * envelope. `limit` is clamped to [1, 100] in the handler; above 1000 it is
 * rejected outright by ValidationMiddleware's PaginationValidator, which is a
 * different layer with a different answer — both are pinned below.
 *
 * The Hurl file asserted `jsonpath "$" exists` on two of the three endpoints,
 * which passes for any JSON at all including an error envelope. Here every
 * response goes through `cursorPageSchema`, whose refinement carries the one
 * invariant that has to hold on every page: a page claiming `has_more` must
 * hand back a usable `next_cursor`.
 */

const CURSOR_ENDPOINTS = [
	{ name: "unread", path: "/v1/feeds/fetch/cursor" },
	{ name: "viewed", path: "/v1/feeds/fetch/viewed/cursor" },
	{ name: "favorites", path: "/v1/feeds/fetch/favorites/cursor" },
] as const;

test.describe("cursor envelope", () => {
	for (const endpoint of CURSOR_ENDPOINTS) {
		test(`${endpoint.name} returns a well-formed page`, async ({ rest }) => {
			await expectJsonStatus(
				await rest.get(`${endpoint.path}?limit=20`),
				200,
				cursorPageSchema,
			);
		});

		test(`${endpoint.name} rejects a non-RFC3339 cursor`, async ({ rest }) => {
			// New coverage for two of the three: the Hurl file only probed the
			// unread endpoint's malformed-cursor branch, so the other two could
			// have been parsing cursors with a different (or no) validator.
			await expectStatus(await rest.get(`${endpoint.path}?cursor=not-rfc3339`), 400);
		});

		test(`${endpoint.name} rejects limit=0`, async ({ rest }) => {
			// PaginationValidator: "Limit must be a positive integer".
			await expectStatus(await rest.get(`${endpoint.path}?limit=0`), 400);
		});

		test(`${endpoint.name} rejects a limit over 1000`, async ({ rest }) => {
			await expectStatus(await rest.get(`${endpoint.path}?limit=1001`), 400);
		});
	}
});

test.describe("cursor pagination behaviour", () => {
	test("an oversized but legal limit is clamped, not rejected", async ({ rest }) => {
		// 1000 passes the validator (its ceiling) and is then clamped to 100 by
		// the handler. Two layers, two different numbers — a refactor that
		// collapsed them into one would break exactly here.
		const page = await expectJsonStatus(
			await rest.get("/v1/feeds/fetch/cursor?limit=1000"),
			200,
			cursorPageSchema,
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
			cursorPageSchema,
		);

		test.skip(!first.has_more, "single page of results — nothing to advance to");

		const cursor = first.next_cursor;
		expect(typeof cursor, "has_more implies a cursor").toBe("string");

		const second = await expectJsonStatus(
			await rest.get(`/v1/feeds/fetch/cursor?limit=1&cursor=${encodeURIComponent(cursor ?? "")}`),
			200,
			cursorPageSchema,
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

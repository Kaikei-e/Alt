import type { APIRequestContext } from "@playwright/test";
import { test, expect, callAs } from "../src/fixtures.js";
import { expectJson, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { connectErrorSchema, ConnectCode } from "../../_shared/connect.js";
import { AUGUR_UNARY } from "../src/procedures.js";
import { listConversationsResponseSchema } from "../src/schemas.js";
import type { ListConversationsResponse } from "../src/schemas.js";
import { pagingOrder, users } from "../src/seed.js";

/**
 * Keyset pagination over the history index — entirely new coverage.
 *
 * The Hurl suite called `ListConversations` twice with an empty request and
 * never sent `page_size` or `page_token` at all, so the cursor path
 * (`encodePageToken` / `decodePageToken`, handler.go:844-862, and the
 * `(last_activity_at, id) < ($2, $3)` bound in repo.ListSummaries:162) had no
 * black-box coverage whatsoever. That is the code most likely to silently
 * skip or duplicate a row, and the least likely to be noticed when it does:
 * the SPA's history list just quietly loses a conversation.
 *
 * Every test here is read-only against `users.paging`, which owns three seeded
 * conversations and nothing else touches (src/seed.ts), so they parallelise
 * freely.
 */

const list = async (
	api: APIRequestContext,
	request: Record<string, unknown>,
): Promise<ListConversationsResponse> =>
	expectJsonStatus(
		await callAs(api, AUGUR_UNARY.listConversations, users.paging, request),
		200,
		listConversationsResponseSchema,
	);

const idsOf = (body: ListConversationsResponse): string[] =>
	(body.conversations ?? []).map((c) => c.id);

test.describe("history index ordering", () => {
	test(
		"orders by last activity, newest first",
		{ tag: "@contract" },
		async ({ connect }) => {
			// `ORDER BY last_activity_at DESC, id DESC`. The seeded rows have
			// distinct message timestamps a day apart, so the expected order is
			// total rather than a tie broken by insertion order — which is what
			// makes a lost ORDER BY a failure here instead of a coin flip.
			const body = await list(connect, {});
			expect(idsOf(body)).toEqual([...pagingOrder]);

			// No `page_size`, so `limit` is 0 at the handler and the token is
			// minted only when `len(summaries) == limit` (handler.go:775). Three
			// rows against a requested zero can never satisfy it.
			expect(body.nextPageToken).toBeUndefined();
		},
	);
});

test.describe("cursor paging", () => {
	test(
		"page_size splits the index and the cursor walks the remainder",
		{ tag: "@contract" },
		async ({ connect }) => {
			// The whole walk lives in one test on purpose: the second request
			// consumes a token the first produced, and splitting them would
			// recreate exactly the file-scoped-capture ordering that forced the
			// Hurl runner to `--jobs 1`. Self-contained here, it needs no
			// ordering guarantee at all.
			const first = await list(connect, { pageSize: 2 });
			expect(idsOf(first)).toEqual([pagingOrder[0], pagingOrder[1]]);

			const token = first.nextPageToken;
			expect(
				token,
				"a full page must mint a cursor, or the SPA's history list stops at 2",
			).toBeTruthy();

			const second = await list(connect, { pageSize: 2, pageToken: token });
			// The bound is strict (`<`), so the row the cursor was built from
			// must NOT reappear. An off-by-one here duplicates a conversation on
			// every scroll.
			expect(idsOf(second)).toEqual([pagingOrder[2]]);
			expect(second.nextPageToken).toBeUndefined();
		},
	);

	test(
		"a final page that exactly fills page_size still mints a cursor",
		{ tag: "@contract" },
		async ({ connect }) => {
			// Pinning a rough edge rather than an ideal. The token is minted from
			// `len(summaries) == limit` alone (handler.go:775) — the handler never
			// looks ahead — so a last page that happens to be exactly `page_size`
			// long hands back a cursor that leads nowhere. A client that treats a
			// present token as "there is more" renders one empty page.
			//
			// Asserting it keeps the behaviour visible; if it is ever fixed with a
			// look-ahead, this test is where that shows up.
			const full = await list(connect, { pageSize: 3 });
			expect(idsOf(full)).toEqual([...pagingOrder]);
			const token = full.nextPageToken;
			expect(token).toBeTruthy();

			const beyond = await list(connect, { pageSize: 3, pageToken: token });
			expect(idsOf(beyond)).toEqual([]);
			expect(beyond.nextPageToken).toBeUndefined();
		},
	);

	test(
		"page_size above the 100 cap falls back to the default page size",
		{ tag: "@contract" },
		async ({ connect }) => {
			// `repo.ListSummaries` clamps `limit <= 0 || limit > 100` to 20
			// (augur_conversation_repo.go:147). Without that clamp a caller could
			// ask for the entire table in one query. The observable consequence
			// is subtle and worth pinning: the handler compares the row count
			// against the *requested* page size, not the effective one, so an
            // over-cap request can never mint a cursor either.
			const body = await list(connect, { pageSize: 101 });
			expect(idsOf(body)).toEqual([...pagingOrder]);
			expect(body.nextPageToken).toBeUndefined();
		},
	);
});

test.describe("cursor validation", () => {
	test(
		"an unparsable page_token is invalid_argument",
		{ tag: "@contract" },
		async ({ connect }) => {
			// `decodePageToken` requires `<unix-nanos>|<uuid>` and the handler
			// turns a miss into `invalid_argument` (handler.go:751). The
			// alternative — treating a bad token as "start from the beginning" —
			// would make a corrupted cursor look like an infinite list to a
			// paging client.
			const response = await callAs(connect, AUGUR_UNARY.listConversations, users.paging, {
				pageToken: "not-a-cursor",
			});
			await expectStatus(response, 400);
			const body = await expectJson(response, connectErrorSchema);
			expect(body.code).toBe(ConnectCode.invalidArgument);
		},
	);

	test(
		"a well-shaped page_token with a non-UUID id is invalid_argument",
		{ tag: "@contract" },
		async ({ connect }) => {
			// The two halves of the token are validated separately
			// (handler.go:848-862): this one passes the `|` split and the int64
			// parse and fails only on `uuid.Parse`. Without it, a test on the
			// first branch alone would pass against a decoder that skipped the id
			// check entirely and let a forged cursor through to the query.
			const response = await callAs(connect, AUGUR_UNARY.listConversations, users.paging, {
				pageToken: "1767225600000000000|not-a-uuid",
			});
			await expectStatus(response, 400);
			const body = await expectJson(response, connectErrorSchema);
			expect(body.code).toBe(ConnectCode.invalidArgument);
		},
	);
});

import { test, expect, callAs, procedurePath } from "../src/fixtures.js";
import { expectJson, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { connectErrorSchema, ConnectCode } from "../../_shared/connect.js";
import { AUGUR_UNARY } from "../src/procedures.js";
import {
	deleteConversationResponseSchema,
	getConversationResponseSchema,
	listConversationsResponseSchema,
} from "../src/schemas.js";
import { conversations, history, users } from "../src/seed.js";

/**
 * The `X-Alt-User-Id` trust boundary — the port of
 * `03-augur-missing-user-header.hurl` and `04-augur-malformed-user-header.hurl`,
 * extended with the tenancy negatives the Hurl suite never had.
 *
 * rag-orchestrator has no auth interceptor and validates no JWT: identity is
 * this one header, read per-RPC by `extractUserID` (augur/handler.go:106).
 * Proving the caller really is alt-backend is the *listener's* job, and in this
 * slice `PEER_IDENTITY_MODE=disabled` means there is no such proof — exposure
 * is limited by network policy alone (handler.go:33-40). Two things follow, and
 * both are asserted here:
 *
 *   1. The header must fail **closed**: absent or malformed is
 *      `unauthenticated`, never "treat as anonymous and return everything".
 *   2. Because the header is the *only* identity, every row-level query must
 *      carry it. `repository/augur_conversation_repo.go` does — every SELECT
 *      and the DELETE end in `AND user_id = $2` — and the cross-user tests
 *      below are what keeps that true.
 */

test.describe("the identity header fails closed", () => {
	for (const [name, procedure] of Object.entries(AUGUR_UNARY)) {
		test(`${name} without X-Alt-User-Id is unauthenticated`, { tag: "@authz" }, async ({
			connect,
		}) => {
			const response = await connect.post(procedurePath(procedure), { data: {} });
			await expectStatus(response, 401);
			const body = await expectJson(response, connectErrorSchema);
			expect(body.code).toBe(ConnectCode.unauthenticated);
			// The message is locked to the handler's literal string
			// (handler.go:109). A silent relaxation of the auth contract — say,
			// defaulting to a system user when the header is absent — would keep
			// the 401 for some other reason and this is what catches it.
			expect(body.message).toBe("missing X-Alt-User-Id header");
		});

		test(`${name} with a non-UUID X-Alt-User-Id is unauthenticated`, { tag: "@authz" }, async ({
			connect,
		}) => {
			const response = await callAs(connect, procedure, "not-a-uuid", {});
			await expectStatus(response, 401);
			const body = await expectJson(response, connectErrorSchema);
			expect(body.code).toBe(ConnectCode.unauthenticated);
			// Only the prefix is pinned (handler.go:113): the suffix carries
			// `uuid.Parse`'s error text, which Go's stdlib is free to reword.
			expect(body.message ?? "").toMatch(/^invalid X-Alt-User-Id header:/);
		});
	}

	test(
		"a whitespace-only X-Alt-User-Id is unauthenticated, not a nil user",
		{ tag: "@authz" },
		async ({ connect }) => {
			// New coverage. `extractUserID` trims before the emptiness check, so
			// "   " must take the *missing* branch. Getting this wrong would let
			// `uuid.Parse("   ")` fail into the second branch — same status,
			// different reason — or, if the trim were ever dropped, produce
			// `uuid.Nil` and query the database for user 00000000-…. The message
			// is what distinguishes the three.
			const response = await callAs(connect, AUGUR_UNARY.listConversations, "   ", {});
			await expectStatus(response, 401);
			const body = await expectJson(response, connectErrorSchema);
			expect(body.message).toBe("missing X-Alt-User-Id header");
		},
	);
});

test.describe("one user cannot reach another user's conversation", () => {
	test(
		"GetConversation for someone else's conversation is not_found",
		{ tag: "@authz" },
		async ({ connect }) => {
			// New coverage — the single most valuable negative missing from the
			// Hurl suite. `repo.GetConversation` selects
			// `WHERE id = $1 AND user_id = $2`; drop the second predicate and this
			// call starts returning another user's whole chat history, with no
			// other test in the fleet noticing.
			//
			// `not_found` rather than `permission_denied` is the correct answer
			// and is asserted as such: the handler cannot distinguish "no such
			// row" from "not yours" without leaking the existence of the row.
			const response = await callAs(connect, AUGUR_UNARY.getConversation, users.intruder, {
				id: conversations.history,
			});
			await expectStatus(response, 404);
			const body = await expectJson(response, connectErrorSchema);
			expect(body.code).toBe(ConnectCode.notFound);
		},
	);

	test(
		"ListConversations never leaks another user's rows",
		{ tag: "@authz" },
		async ({ connect }) => {
			// The intruder owns nothing (src/seed.ts), so `ListSummaries`'
			// `WHERE user_id = $1` must produce the empty message — literally
			// `{}` on the wire, since protojson omits empty repeated fields.
			// Asserting emptiness *and* the absence of the seeded ids means the
			// test still fails if the filter is relaxed to something that returns
			// rows without returning all of them.
			const response = await callAs(connect, AUGUR_UNARY.listConversations, users.intruder, {});
			const body = await expectJsonStatus(response, 200, listConversationsResponseSchema);
			expect(body.conversations ?? []).toEqual([]);
		},
	);

	test(
		"DeleteConversation does not destroy someone else's conversation",
		{ tag: "@authz" },
		async ({ connect }) => {
			// The sharpest failure this suite guards. `repo.DeleteConversation`
			// answers 200 whether or not it removed a row — that idempotency is
			// the documented contract (see augur-history.spec.ts) — so a dropped
			// `AND user_id = $2` would let any caller delete any conversation and
			// still get a success. The status alone proves nothing; the owner's
			// read-back is the assertion.
			//
			// The target is a conversation seeded for exactly this test
			// (`protectedOwner` in src/seed.ts), so a regression destroys one row
			// nothing else reads instead of taking the history fixtures with it.
			const attempt = await callAs(connect, AUGUR_UNARY.deleteConversation, users.intruder, {
				id: conversations.protected,
			});
			await expectJsonStatus(attempt, 200, deleteConversationResponseSchema);

			const owned = await callAs(connect, AUGUR_UNARY.getConversation, users.protectedOwner, {
				id: conversations.protected,
			});
			const body = await expectJsonStatus(owned, 200, getConversationResponseSchema);
			expect(
				body.messages ?? [],
				"the intruder's delete removed the owner's turns — the user_id " +
					"predicate in repo.DeleteConversation is gone",
			).toHaveLength(2);
		},
	);

	test(
		"a seeded conversation is invisible to the empty user",
		{ tag: "@authz" },
		async ({ connect }) => {
			// The mirror of the history read in augur-history.spec.ts, from the
			// other side. `history.title` is only ever returned to `users.history`;
			// this asserts the same id under a different caller yields nothing,
			// which is what makes the positive read a *scoped* read rather than a
			// global one.
			const response = await callAs(connect, AUGUR_UNARY.getConversation, users.empty, {
				id: conversations.history,
			});
			await expectStatus(response, 404);
			expect(
				await response.text(),
				`the not_found body must not leak "${history.title}"`,
			).not.toContain(history.title);
		},
	);
});

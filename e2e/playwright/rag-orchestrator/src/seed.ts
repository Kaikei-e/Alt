/**
 * The identities `setup/db-seed.sql` writes, and who owns what.
 *
 * Parallel safety in this suite comes from **ownership**, not from naming: no
 * Augur write RPC exists that a test could seed through (`StreamChat` is the
 * only creator and it drives the whole RAG pipeline), so rows arrive out of
 * band and each `user_id` below is exercised by exactly one group of tests.
 * `augur_conversations` and `augur_messages` are both scoped by `user_id` at
 * the repository layer (`repository/augur_conversation_repo.go`: every SELECT
 * and the DELETE carry `AND user_id = $2`), so two owners cannot see each
 * other's rows even under six workers.
 *
 * | Owner              | Rows                              | Touched by |
 * |--------------------|-----------------------------------|------------|
 * | `historyUser`      | 1 conversation, 2 turns           | tests/augur-history.spec.ts (read-only) |
 * | `emptyUser`        | none                              | the empty-list / not-found / unknown-id negatives (creates nothing) |
 * | `pagingUser`       | 3 conversations, 1 turn each      | tests/augur-pagination.spec.ts (read-only) |
 * | `deletableOwner`   | 1 conversation, 1 turn            | the one destructive test |
 * | `protectedOwner`   | 1 conversation, 2 turns           | the cross-user delete negative, then read back by its owner |
 * | `intruderUser`     | none                              | the cross-user negatives; owns nothing so its own list stays empty |
 *
 * Editing this file means editing `setup/db-seed.sql` in the same change.
 */

/** `X-Alt-User-Id` values. */
export const users = {
	history: "00000000-0000-0000-0000-00000e2e0001",
	empty: "00000000-0000-0000-0000-00000e2e0002",
	paging: "00000000-0000-0000-0000-00000e2e0003",
	deletableOwner: "00000000-0000-0000-0000-00000e2e0004",
	protectedOwner: "00000000-0000-0000-0000-00000e2e0005",
	intruder: "00000000-0000-0000-0000-00000e2e0006",
} as const;

/** Seeded `augur_conversations.id` values. */
export const conversations = {
	history: "0e2e0001-0000-0000-0000-000000000001",
	paging1: "0e2e0002-0000-0000-0000-000000000001",
	paging2: "0e2e0002-0000-0000-0000-000000000002",
	paging3: "0e2e0002-0000-0000-0000-000000000003",
	deletable: "0e2e0003-0000-0000-0000-000000000001",
	protected: "0e2e0004-0000-0000-0000-000000000001",
} as const;

/** The exact strings the seeded history conversation carries. */
export const history = {
	title: "E2E seed conversation",
	userTurn: "What did the RSS reader log yesterday?",
	assistantTurn: "Seeded assistant reply for E2E.",
	/** `kind: "article"` citation — `ref_id` is what the SPA routes to `/articles/<id>`. */
	articleRefId: "0e2e9001-0000-0000-0000-000000000001",
	/** `kind: "web"` citation. */
	webURL: "https://example.invalid/e2e-seed-web",
	/**
	 * A third citation whose stored `title` is literally this UUID. The read
	 * path must blank it (`sanitizeCitationTitle`, augur/handler.go:539) — the
	 * ADR-926 label-fallback bug, where the UI rendered a raw internal id as
	 * the visible source name.
	 */
	uuidTitledRefId: "0e2e9001-0000-0000-0000-000000000002",
	relatedURL: "https://example.invalid/e2e-seed-related",
} as const;

/** Paging conversations, newest last-activity first — the order the view sorts them in. */
export const pagingOrder = [
	conversations.paging3,
	conversations.paging2,
	conversations.paging1,
] as const;

/**
 * A syntactically valid UUID that is deliberately not a seeded row.
 *
 * Not `ZERO_UUID`: the all-zero value is special-cased by
 * `augurConversationUsecase.GetConversation`, which rejects `uuid.Nil` before
 * it reaches the query. That distinction is asserted on purpose in
 * tests/augur-history.spec.ts, so the ordinary "unknown id" probes need an id
 * that really does reach the database.
 */
export const UNKNOWN_CONVERSATION_ID = "99999999-9999-9999-9999-999999999999";

/** A second unknown id, so the delete-idempotency probe never collides with the read probe. */
export const UNKNOWN_CONVERSATION_ID_2 = "99999999-9999-9999-9999-999999999998";

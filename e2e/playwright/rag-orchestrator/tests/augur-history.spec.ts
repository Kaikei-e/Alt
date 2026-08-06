import { test, expect, callAs } from "../src/fixtures.js";
import { expectJson, expectJsonStatus, expectStatus } from "../../_shared/http.js";
import { connectErrorSchema, ConnectCode } from "../../_shared/connect.js";
import { ZERO_UUID } from "../../_shared/env.js";
import { AUGUR_UNARY } from "../src/procedures.js";
import {
	deleteConversationResponseSchema,
	getConversationResponseSchema,
	listConversationsResponseSchema,
} from "../src/schemas.js";
import {
	UNKNOWN_CONVERSATION_ID,
	UNKNOWN_CONVERSATION_ID_2,
	conversations,
	history,
	users,
} from "../src/seed.js";

/**
 * Augur chat history read paths — the port of `05-augur-list-empty.hurl`
 * through `09-augur-get-seeded.hurl`.
 *
 * The rows come from `setup/db-seed.sql` rather than from a test, because Augur
 * has no write RPC that does not drive the whole RAG pipeline (`StreamChat` is
 * the only creator). What replaces per-test seeding is per-role **ownership**:
 * every `user_id` in `src/seed.ts` belongs to one group of tests, so six
 * workers reading and deleting concurrently never contend. The one destructive
 * test owns a conversation nothing else reads, and asserts only idempotent
 * facts so a CI retry re-runs it cleanly.
 */

test.describe("the history index", () => {
	test(
		"a user with no conversations gets the empty message",
		{ tag: "@contract" },
		async ({ connect }) => {
			const response = await callAs(connect, AUGUR_UNARY.listConversations, users.empty, {});
			const body = await expectJsonStatus(response, 200, listConversationsResponseSchema);

			// Both halves matter and the Hurl file asserted both. protojson omits
			// empty repeated fields and empty strings, so a correct empty page is
			// literally the two bytes `{}` — no `conversations` key, no
			// `nextPageToken`. A handler that started emitting
			// `{"conversations": []}` would be a wire change the SPA's generated
			// client absorbs silently but a proxy or a cache key might not.
			expect(body.conversations).toBeUndefined();
			expect(body.nextPageToken).toBeUndefined();
			expect(await response.text()).toBe("{}");
		},
	);

	test(
		"returns the seeded conversation with its derived counters",
		{ tag: "@contract" },
		async ({ connect }) => {
			const response = await callAs(connect, AUGUR_UNARY.listConversations, users.history, {});
			const body = await expectJsonStatus(response, 200, listConversationsResponseSchema);

			const rows = body.conversations ?? [];
			expect(rows).toHaveLength(1);
			const row = rows[0];
			if (row === undefined) throw new Error("unreachable: length asserted above");

			expect(row.id).toBe(conversations.history);
			expect(row.title).toBe(history.title);
			// `messageCount` and `lastMessagePreview` are not columns: the
			// `augur_conversation_index` view derives them from `augur_messages`
			// with a LATERAL join (migrations/20260413120000). Asserting them is
			// the only black-box check that the disposable projection is still
			// being computed rather than quietly returning conversation defaults.
			expect(row.messageCount).toBe(2);
			expect(row.lastMessagePreview).toBe(history.assistantTurn);

			// One page, so no cursor. The token is only minted when the page came
			// back exactly `limit` long (handler.go:775) — see
			// tests/augur-pagination.spec.ts for that boundary.
			expect(body.nextPageToken).toBeUndefined();
		},
	);

	test(
		"lastActivityAt tracks the newest turn, not the conversation row",
		{ tag: "@contract" },
		async ({ connect }) => {
			// New coverage. The view COALESCEs `MAX(augur_messages.created_at)`
			// over the conversation's own `created_at`, and `last_activity_at` is
			// the primary sort key *and* half of the keyset cursor
			// (repo.ListSummaries:162). If it silently fell back to `created_at`
			// the history list would stop reordering as a chat continued, and the
			// cursor would start skipping rows — neither of which any assertion
			// on `title` or `messageCount` would catch.
			const body = await expectJsonStatus(
				await callAs(connect, AUGUR_UNARY.listConversations, users.history, {}),
				200,
				listConversationsResponseSchema,
			);
			const row = (body.conversations ?? [])[0];
			if (row === undefined) throw new Error("the seeded history conversation is missing");

			expect(row.createdAt).toBe("2026-01-01T00:00:00Z");
			expect(row.lastActivityAt).toBe("2026-01-01T00:00:02Z");
		},
	);
});

test.describe("reading a single conversation", () => {
	test(
		"returns both turns in insertion order",
		{ tag: "@contract" },
		async ({ connect }) => {
			const body = await expectJsonStatus(
				await callAs(connect, AUGUR_UNARY.getConversation, users.history, {
					id: conversations.history,
				}),
				200,
				getConversationResponseSchema,
			);

			expect(body.id).toBe(conversations.history);
			expect(body.title).toBe(history.title);

			const messages = body.messages ?? [];
			expect(messages).toHaveLength(2);
			// `ORDER BY created_at ASC, id ASC` (repo.ListMessages:106). Order is
			// the contract: the SPA renders the array as-is, so a lost ORDER BY
			// shows up as an answer appearing above its question.
			expect(messages.map((m) => m.role)).toEqual(["user", "assistant"]);
			expect(messages.map((m) => m.content)).toEqual([
				history.userTurn,
				history.assistantTurn,
			]);
		},
	);

	test(
		"rebuilds the citation kind discriminator from stored JSONB",
		{ tag: "@contract" },
		async ({ connect }) => {
			// New coverage. Citations are persisted as JSONB with a lowercase
			// domain string (`"article"` / `"web"`), and the read path has to
			// translate them back to the proto enum through `protoCitationKind`
			// (handler.go:655) before `domainCitationsToProto` writes the wire
			// form. Get that mapping wrong and every stored citation comes back
			// `CITATION_KIND_UNSPECIFIED`, which the SPA renders as a *disabled
			// span* — the links silently stop working with no error anywhere.
			const body = await expectJsonStatus(
				await callAs(connect, AUGUR_UNARY.getConversation, users.history, {
					id: conversations.history,
				}),
				200,
				getConversationResponseSchema,
			);
			const assistant = (body.messages ?? [])[1];
			if (assistant === undefined) throw new Error("the assistant turn is missing");

			const citations = assistant.citations ?? [];
			expect(citations).toHaveLength(3);

			const [article, web, uuidTitled] = citations;
			if (article === undefined || web === undefined || uuidTitled === undefined) {
				throw new Error("unreachable: length asserted above");
			}

			expect(article.kind).toBe("CITATION_KIND_ARTICLE");
			// `ref_id` is what the SPA routes to /articles/<id>; for an ARTICLE
			// citation `url` is meaningless and stays empty (hence omitted).
			expect(article.refId).toBe(history.articleRefId);
			expect(article.url ?? "").toBe("");

			expect(web.kind).toBe("CITATION_KIND_WEB");
			expect(web.url).toBe(history.webURL);
		},
	);

	test(
		"blanks a citation title that is a bare UUID",
		{ tag: "@contract" },
		async ({ connect }) => {
			// New coverage, and a regression fence for ADR-926: a citation whose
			// stored title is just an internal identifier used to be rendered as
			// the visible source name. `sanitizeCitationTitle` (handler.go:539)
			// blanks it so the SPA falls back to the URL's domain or "Untitled
			// source". The seed row carries exactly that shape.
			//
			// Blank means *absent* on the wire: protojson omits an empty string.
			const body = await expectJsonStatus(
				await callAs(connect, AUGUR_UNARY.getConversation, users.history, {
					id: conversations.history,
				}),
				200,
				getConversationResponseSchema,
			);
			const uuidTitled = (body.messages ?? [])[1]?.citations?.[2];
			if (uuidTitled === undefined) throw new Error("the UUID-titled citation is missing");

			expect(uuidTitled.refId).toBe(history.uuidTitledRefId);
			expect(
				uuidTitled.title ?? "",
				"a UUID leaked into the citation's visible title (ADR-926)",
			).toBe("");
		},
	);

	test(
		"returns the related-citations snapshot alongside the direct ones",
		{ tag: "@contract" },
		async ({ connect }) => {
			// New coverage. `related_citations` is a separate JSONB column added
			// by migrations/20260527100000 and read back through its own
			// `domainCitationsToProto` call (handler.go:812). It is an
			// inline-projected snapshot fixed at write time and never backfilled,
			// so a read path that dropped it would look exactly like "this turn
			// predates the feature" — indistinguishable from the real thing
			// without a row that definitely has one.
			const body = await expectJsonStatus(
				await callAs(connect, AUGUR_UNARY.getConversation, users.history, {
					id: conversations.history,
				}),
				200,
				getConversationResponseSchema,
			);
			const messages = body.messages ?? [];
			const assistant = messages[1];
			if (assistant === undefined) throw new Error("the assistant turn is missing");

			expect(assistant.relatedCitations ?? []).toHaveLength(1);
			expect(assistant.relatedCitations?.[0]?.url).toBe(history.relatedURL);

			// The user turn carries neither list, which is what proves the two
			// columns are read per-row rather than broadcast from the last one.
			expect(messages[0]?.citations ?? []).toEqual([]);
			expect(messages[0]?.relatedCitations ?? []).toEqual([]);
		},
	);

	test(
		"an unknown but well-formed id is not_found, not internal",
		{ tag: "@contract" },
		async ({ connect }) => {
			// `repo.GetConversation` maps `pgx.ErrNoRows` to `(nil, nil)` and the
			// handler turns that into `CodeNotFound` (handler.go:800). The
			// distinction is the whole test: a missing row surfacing as
			// `internal` would page an operator every time a user opened a stale
			// bookmark.
			const response = await callAs(connect, AUGUR_UNARY.getConversation, users.empty, {
				id: UNKNOWN_CONVERSATION_ID,
			});
			await expectStatus(response, 404);
			const body = await expectJson(response, connectErrorSchema);
			expect(body.code).toBe(ConnectCode.notFound);
		},
	);

	test(
		"a non-UUID id is invalid_argument, not not_found",
		{ tag: "@contract" },
		async ({ connect }) => {
			// New coverage. `uuid.Parse` fails before any query
			// (handler.go:791), so this is the caller's fault and must say so —
			// collapsing it into `not_found` would tell a client "that
			// conversation is gone" when the truth is "your id is malformed", and
			// the SPA acts on those very differently.
			const response = await callAs(connect, AUGUR_UNARY.getConversation, users.empty, {
				id: "not-a-uuid",
			});
			await expectStatus(response, 400);
			const body = await expectJson(response, connectErrorSchema);
			expect(body.code).toBe(ConnectCode.invalidArgument);
		},
	);

	test(
		"the all-zero id is rejected before it reaches the database",
		{ tag: "@contract" },
		async ({ connect }) => {
			// New coverage, and a deliberate pin on a rough edge.
			// `00000000-…-000000000000` parses as a UUID, so it clears the
			// handler's guard, but `augurConversationUsecase.GetConversation`
			// refuses `uuid.Nil` explicitly and returns a plain error — which
			// `handler.go:798` maps to `CodeInternal`, i.e. HTTP 500.
			//
			// The behaviour under test is "the zero UUID never becomes a query",
			// which is right; the *code* it surfaces as is arguably wrong
			// (`invalid_argument` would be the honest answer). Pinning it means a
			// future correction is a deliberate, visible change here rather than
			// a silent one.
			const response = await callAs(connect, AUGUR_UNARY.getConversation, users.empty, {
				id: ZERO_UUID,
			});
			await expectStatus(response, 500);
			const body = await expectJson(response, connectErrorSchema);
			expect(body.code).toBe(ConnectCode.internal);
		},
	);
});

test.describe("deleting a conversation", () => {
	test(
		"an unknown id is idempotent, not not_found",
		{ tag: "@contract" },
		async ({ connect }) => {
			// The handler's current contract (handler.go:825-841): the DELETE
			// affects zero rows and the response is the empty message. If a
			// future refactor tightens it to `not_found` this flips to RED, which
			// is the desired breadcrumb — the SPA's "delete chat" button retries,
			// and a 404 on the retry would surface as an error to the user for a
			// delete that already succeeded.
			//
			// `.strict()` on the schema is what carries `07`'s
			// `jsonpath "$.code" not exists`: a Connect error envelope arriving
			// under a 200 would fail here.
			const response = await callAs(connect, AUGUR_UNARY.deleteConversation, users.empty, {
				id: UNKNOWN_CONVERSATION_ID_2,
			});
			await expectJsonStatus(response, 200, deleteConversationResponseSchema);
			expect(await response.text()).toBe("{}");
		},
	);

	test(
		"a non-UUID id is invalid_argument",
		{ tag: "@contract" },
		async ({ connect }) => {
			// New coverage. The idempotent-success contract above makes this one
			// load-bearing: if a malformed id also answered 200, a client bug that
			// sent garbage would look like a successful delete forever.
			const response = await callAs(connect, AUGUR_UNARY.deleteConversation, users.empty, {
				id: "not-a-uuid",
			});
			await expectStatus(response, 400);
			const body = await expectJson(response, connectErrorSchema);
			expect(body.code).toBe(ConnectCode.invalidArgument);
		},
	);

	test(
		"removes the owner's conversation and its turns",
		{ tag: "@contract" },
		async ({ connect }) => {
			// The suite's only destructive test. It owns
			// `conversations.deletable` outright (src/seed.ts), so it cannot race
			// a sibling worker, and every assertion below is idempotent so a CI
			// retry re-runs cleanly against the already-deleted row. The
			// pre-state — that the row existed at all — is established once, for
			// the whole run, by the seed probe in setup/global-setup.ts; asserting
			// it here instead would make the retry fail for the wrong reason.
			await expectJsonStatus(
				await callAs(connect, AUGUR_UNARY.deleteConversation, users.deletableOwner, {
					id: conversations.deletable,
				}),
				200,
				deleteConversationResponseSchema,
			);

			await expectStatus(
				await callAs(connect, AUGUR_UNARY.getConversation, users.deletableOwner, {
					id: conversations.deletable,
				}),
				404,
			);

			// The turns go with it: `augur_messages.conversation_id` is
			// `REFERENCES augur_conversations(id) ON DELETE CASCADE`
			// (migrations/20260413120000), and the repository issues no message
			// delete of its own. An empty history index for the owner is the
			// black-box shape of "the parent row and everything hanging off it
			// are gone".
			const after = await expectJsonStatus(
				await callAs(connect, AUGUR_UNARY.listConversations, users.deletableOwner, {}),
				200,
				listConversationsResponseSchema,
			);
			expect(after.conversations ?? []).toEqual([]);

			// Idempotent on repeat, which is also what makes this test
			// retry-safe.
			await expectJsonStatus(
				await callAs(connect, AUGUR_UNARY.deleteConversation, users.deletableOwner, {
					id: conversations.deletable,
				}),
				200,
				deleteConversationResponseSchema,
			);
		},
	);
});

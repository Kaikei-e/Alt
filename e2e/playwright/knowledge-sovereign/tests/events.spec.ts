import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { expectUnaryError } from "../../_shared/connect.js";
import { uuid } from "../../_shared/ids.js";
import { procedure } from "../src/env.js";
import { appendEvent, appendEventBody, encodePayload, instant } from "../src/fixtures.js";
import {
	appendEventSchema,
	knowledgeEventSchema,
	latestEventSeqSchema,
	listEventsSchema,
} from "../src/schemas.js";
import { z } from "zod";

/**
 * The append-only event log — `knowledge_events`, the one table in Alt that is
 * INSERT-only by invariant (CLAUDE.md, immutable data model).
 *
 * Ports `03-event-append-happy.hurl`, `04-event-latest-seq.hurl` and
 * `05-event-list-tenant-scoped.hurl`, and adds the tenant boundary, the
 * dedupe/idempotency contract, and the cursor edges — none of which the Hurl
 * suite touched. Its own README listed "ListKnowledgeEvents cross-tenant
 * reject" and "AppendKnowledgeEvent invalid payload" under **Out of scope
 * (deferred)**; both are here now.
 *
 * Every test seeds under its own `principal.userId`, so "exactly one event"
 * is a claim about the driver's predicate rather than about the schedule. The
 * Hurl suite could only make that claim with `--jobs 1`.
 */

/** Decodes a proto3-JSON `bytes` field back to the object that was sent. */
function decodePayload(base64: string): unknown {
	return JSON.parse(Buffer.from(base64, "base64").toString("utf8"));
}

test.describe("append", () => {
	test(
		"AppendKnowledgeEvent returns the assigned event_seq",
		{ tag: "@smoke" },
		async ({ rpc, principal }) => {
			const { occurredAt } = instant();
			const eventId = uuid();
			const response = await rpc.post(procedure("AppendKnowledgeEvent"), {
				data: appendEventBody({
					tenantId: principal.tenantId,
					userId: principal.userId,
					eventType: "E2EProbe",
					aggregateType: "article",
					aggregateId: `article:${uuid()}`,
					dedupeKey: `${principal.token}-append`,
					occurredAt,
					eventId,
					payload: { title: "playwright-e2e" },
				}),
			});

			// `03-event-append-happy.hurl` asserted `matches "^[0-9]+$"`. The
			// schema keeps the string-encoded-int64 fact (protojson renders
			// int64 as a JSON string) and adds the part that was missing: 0 is
			// not a valid answer here. `AppendKnowledgeEvent` returns seq 0 to
			// mean "duplicate, nothing written", so a regex that accepts "0"
			// would pass for an append that silently did nothing.
			const body = await expectJsonStatus(response, 200, appendEventSchema);
			expect(Number(body.eventSeq)).toBeGreaterThan(0);
		},
	);

	test(
		"a repeated dedupe_key is idempotent and writes nothing",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage, and the single most consequential contract on this
			// RPC: `knowledge_event_dedupes` holds the global UNIQUE on
			// dedupe_key precisely so an at-least-once producer — every Redis
			// Streams consumer in Alt, which re-delivers after a crash between
			// the write and the XACK (CLAUDE.md rule 10) — can resend safely.
			//
			// The driver signals the duplicate by returning seq **0**, and
			// protojson omits a zero int64, so the wire answer is the empty
			// object. A caller that treated `{}` as a failure, or a driver
			// that started returning the original seq instead, would both
			// break producers here rather than in production.
			const { occurredAt } = instant();
			const body = appendEventBody({
				tenantId: principal.tenantId,
				userId: principal.userId,
				eventType: "E2EProbe",
				aggregateId: `article:${uuid()}`,
				dedupeKey: `${principal.token}-dedupe`,
				occurredAt,
				payload: { attempt: 1 },
			});

			const first = await expectJsonStatus(
				await rpc.post(procedure("AppendKnowledgeEvent"), { data: body }),
				200,
				appendEventSchema,
			);

			// A *different* event_id and payload, the same dedupe_key: exactly
			// what a redelivered message looks like once the producer has
			// regenerated its envelope.
			const replay = await rpc.post(procedure("AppendKnowledgeEvent"), {
				data: appendEventBody({
					tenantId: principal.tenantId,
					userId: principal.userId,
					eventType: "E2EProbe",
					aggregateId: `article:${uuid()}`,
					dedupeKey: `${principal.token}-dedupe`,
					occurredAt,
					payload: { attempt: 2 },
				}),
			});
			expect(replay.status()).toBe(200);
			expect(
				await replay.json(),
				"a duplicate append must answer seq 0, which protojson omits entirely",
			).toEqual({});

			// And the log itself must still hold one row: the dedupe registry
			// and the event insert run in one transaction so a crash between
			// them cannot register the key without the event.
			const listed = await expectJsonStatus(
				await rpc.post(procedure("ListKnowledgeEvents"), {
					data: { afterSeq: "0", limit: 10, tenantId: principal.tenantId, userId: principal.userId },
				}),
				200,
				z.object({ events: z.array(knowledgeEventSchema).length(1) }).passthrough(),
			);
			expect(listed.events[0]?.eventSeq).toBe(first.eventSeq);
			expect(decodePayload(listed.events[0]?.payload ?? "")).toEqual({ attempt: 1 });
		},
	);

	test(
		"AppendKnowledgeEvent rejects a malformed tenant_id",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage — the Hurl README deferred it. `parseUUIDField`
			// exists so this can never be coerced to uuid.Nil: a Nil tenant on
			// an INSERT-only table is unrecoverable, because the row cannot be
			// updated to fix it and the tenant predicate would hand it to
			// whoever queries tenant Nil.
			const { occurredAt } = instant();
			const error = await expectUnaryError(
				rpc,
				procedure("AppendKnowledgeEvent"),
				{
					event: {
						eventId: uuid(),
						occurredAt,
						tenantId: "not-a-uuid",
						userId: principal.userId,
						eventType: "E2EProbe",
						dedupeKey: `${principal.token}-bad-tenant`,
					},
				},
				"invalid_argument",
			);
			expect(error.message ?? "").toContain("tenant_id");
		},
	);

	test(
		"AppendKnowledgeEvent requires occurred_at",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage. `occurred_at` is the partition key of
			// `knowledge_events` (migration 00006) and the sole time source
			// every projector folds from — `foldArticleCreated` derives the
			// item's freshness score from it rather than from wall clock, so
			// that a reproject is bit-identical. An event with no occurred_at
			// has no defensible default, hence the explicit guard rather than
			// a `time.Now()` fallback.
			await expectUnaryError(
				rpc,
				procedure("AppendKnowledgeEvent"),
				{
					event: {
						eventId: uuid(),
						tenantId: principal.tenantId,
						userId: principal.userId,
						eventType: "E2EProbe",
						dedupeKey: `${principal.token}-no-occurred-at`,
					},
				},
				"invalid_argument",
			);
		},
	);
});

test.describe("read back", () => {
	test(
		"GetLatestEventSeq returns the seq of the event just appended",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// `04-event-latest-seq.hurl` captured `event_seq` in scenario 03
			// and then asserted only `matches "^[0-9]+$"` here — it never
			// compared the two. Its README claimed "the response returns the
			// same event_seq", but the assertion did not check that, so a
			// handler answering with *any* row's sequence would have passed.
			// This is the assertion that description meant.
			const { occurredAt } = instant();
			const seq = await appendEvent(rpc, {
				tenantId: principal.tenantId,
				userId: principal.userId,
				eventType: "E2EProbe",
				aggregateId: `article:${uuid()}`,
				dedupeKey: `${principal.token}-latest`,
				occurredAt,
			});

			const body = await expectJsonStatus(
				await rpc.post(procedure("GetLatestEventSeq"), {
					data: { tenantId: principal.tenantId, userId: principal.userId },
				}),
				200,
				latestEventSeqSchema,
			);
			expect(body.eventSeq).toBe(seq);
		},
	);

	test(
		"GetLatestEventSeq answers 0 for a principal with no events",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage. The driver COALESCEs MAX(event_seq) to 0, and
			// protojson omits a zero int64 — so "no events" is the empty
			// object, not a 404 and not `{"eventSeq":"0"}`. A consumer using
			// this as a resume cursor reads the absent field as 0 and replays
			// from the beginning, which is the correct behaviour and worth
			// pinning.
			const response = await rpc.post(procedure("GetLatestEventSeq"), {
				data: { tenantId: principal.tenantId, userId: uuid() },
			});
			expect(response.status()).toBe(200);
			expect(await response.json()).toEqual({});
		},
	);

	test(
		"ListKnowledgeEvents returns exactly the principal's event, payload intact",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// Ports `05-event-list-tenant-scoped.hurl`, which asserted the
			// count plus five scalar fields. Two things are added:
			//
			//  - the whole envelope through a schema, so a field that changes
			//    type (event_seq becoming a JSON number, say — a real risk
			//    whenever protojson options are touched) fails here;
			//  - the payload round trip. The Hurl file sent a base64 body and
			//    never read it back, so a driver that dropped or truncated the
			//    `payload` column was invisible. `payload` is the only place a
			//    projector may read a business fact from (reproject-safety), so
			//    losing it silently is the worst available failure.
			const { occurredAt } = instant();
			const eventId = uuid();
			const articleId = uuid();
			const payload = { title: "playwright-e2e", article_id: articleId };
			const seq = await appendEvent(rpc, {
				tenantId: principal.tenantId,
				userId: principal.userId,
				eventType: "E2EProbe",
				aggregateType: "article",
				aggregateId: `article:${articleId}`,
				dedupeKey: `${principal.token}-list`,
				occurredAt,
				eventId,
				payload,
			});

			const body = await expectJsonStatus(
				await rpc.post(procedure("ListKnowledgeEvents"), {
					data: {
						afterSeq: "0",
						limit: 10,
						tenantId: principal.tenantId,
						userId: principal.userId,
					},
				}),
				200,
				z.object({ events: z.array(knowledgeEventSchema).length(1) }).passthrough(),
			);

			const event = body.events[0];
			expect(event?.eventId).toBe(eventId);
			expect(event?.eventSeq).toBe(seq);
			expect(event?.eventType).toBe("E2EProbe");
			expect(event?.tenantId).toBe(principal.tenantId);
			expect(event?.userId).toBe(principal.userId);
			expect(event?.dedupeKey).toBe(`${principal.token}-list`);
			expect(event?.aggregateId).toBe(`article:${articleId}`);
			expect(decodePayload(event?.payload ?? "")).toEqual(payload);
			expect(event?.payload).toBe(encodePayload(payload));
		},
	);

	test(
		"after_seq excludes everything at or below the cursor",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage. `WHERE event_seq > $1` is what every projector and
			// every stream consumer uses to resume, so an off-by-one here means
			// either a permanently re-folded event or a permanently skipped
			// one. The Hurl suite only ever passed `after_seq: 0`.
			const { occurredAt } = instant();
			const first = await appendEvent(rpc, {
				tenantId: principal.tenantId,
				userId: principal.userId,
				eventType: "E2EProbe",
				aggregateId: `article:${uuid()}`,
				dedupeKey: `${principal.token}-cursor-1`,
				occurredAt,
			});
			const second = await appendEvent(rpc, {
				tenantId: principal.tenantId,
				userId: principal.userId,
				eventType: "E2EProbe",
				aggregateId: `article:${uuid()}`,
				dedupeKey: `${principal.token}-cursor-2`,
				occurredAt,
			});
			expect(
				Number(second),
				"event_seq comes from a sequence, so the second append must sort after the first",
			).toBeGreaterThan(Number(first));

			const page = await expectJsonStatus(
				await rpc.post(procedure("ListKnowledgeEvents"), {
					data: {
						afterSeq: first,
						limit: 10,
						tenantId: principal.tenantId,
						userId: principal.userId,
					},
				}),
				200,
				z.object({ events: z.array(knowledgeEventSchema).length(1) }).passthrough(),
			);
			expect(page.events[0]?.eventSeq).toBe(second);
		},
	);

	test("limit bounds the page", { tag: "@contract" }, async ({ rpc, principal }) => {
		// New coverage. `limit` reaches the SQL directly as `LIMIT $4` with no
		// clamp, so this is also the assertion that a caller cannot accidentally
		// stream the whole log by asking for one page.
		const { occurredAt } = instant();
		for (const n of [1, 2, 3]) {
			await appendEvent(rpc, {
				tenantId: principal.tenantId,
				userId: principal.userId,
				eventType: "E2EProbe",
				aggregateId: `article:${uuid()}`,
				dedupeKey: `${principal.token}-limit-${n}`,
				occurredAt,
			});
		}

		const page = await expectJsonStatus(
			await rpc.post(procedure("ListKnowledgeEvents"), {
				data: {
					afterSeq: "0",
					limit: 2,
					tenantId: principal.tenantId,
					userId: principal.userId,
				},
			}),
			200,
			z.object({ events: z.array(knowledgeEventSchema).length(2) }).passthrough(),
		);
		// ORDER BY event_seq ASC — the page is the *oldest* two, which is what
		// a resuming consumer needs. A handler that started ordering DESC would
		// make every consumer skip forward and never catch up.
		expect(Number(page.events[0]?.eventSeq)).toBeLessThan(Number(page.events[1]?.eventSeq));
	});
});

test.describe("tenant boundary (ADR-000749)", () => {
	test(
		"another tenant cannot see this principal's events",
		{ tag: "@authz" },
		async ({ rpc, principal }) => {
			// New coverage — listed as deferred in the Hurl README. The tenant
			// predicate is the primary authorization gate on this read
			// (`ListKnowledgeEventsSinceForUser`), and nothing else in the
			// service enforces it: this listener has no authentication at all,
			// so the query's WHERE clause *is* the boundary.
			const { occurredAt } = instant();
			await appendEvent(rpc, {
				tenantId: principal.tenantId,
				userId: principal.userId,
				eventType: "E2EProbe",
				aggregateId: `article:${uuid()}`,
				dedupeKey: `${principal.token}-tenant-boundary`,
				occurredAt,
			});

			const otherTenant = uuid();
			const response = await rpc.post(procedure("ListKnowledgeEvents"), {
				data: { afterSeq: "0", limit: 10, tenantId: otherTenant, userId: principal.userId },
			});
			expect(response.status()).toBe(200);
			// protojson omits an empty repeated field, so "no rows" is `{}` —
			// asserting the exact document rather than `events.length === 0`
			// also catches a handler that started returning `null`.
			expect(await response.json()).toEqual({});
		},
	);

	test(
		"user_id without tenant_id is rejected rather than answered tenant-wide",
		{ tag: "@authz" },
		async ({ rpc, principal }) => {
			// New coverage. The guard is explicit in `ListKnowledgeEvents`:
			// with `user_id` set and `tenant_id` empty, the handler must not
			// fall through to the unscoped `ListKnowledgeEventsSince` branch —
			// which is the projector-only path over *every* tenant's log. The
			// fall-through would be a cross-tenant read that returns 200.
			const error = await expectUnaryError(
				rpc,
				procedure("ListKnowledgeEvents"),
				{ afterSeq: "0", limit: 10, userId: principal.userId },
				"invalid_argument",
			);
			expect(error.message ?? "").toContain("tenant_id");
		},
	);

	test(
		"a tenant-wide event with no user_id is visible to every user in the tenant",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage for behaviour that reads like a bug and is not:
			// `(user_id = $3 OR user_id IS NULL)` deliberately admits
			// tenant-wide system events — ArticleCreated is emitted once per
			// tenant, and the home projector resolves its owner with
			// `resolveUserID`, which falls back to the tenant id. Pinning it
			// stops someone "tightening" the predicate and silently emptying
			// every user's Knowledge Home.
			//
			// This test mints its own tenant rather than using the worker's:
			// a user_id-less row under the shared tenant would be visible to
			// every other test in this worker and would break their
			// exactly-one-event counts.
			const isolatedTenant = uuid();
			const { occurredAt } = instant();
			const dedupeKey = `${principal.token}-tenant-wide`;
			await appendEvent(rpc, {
				tenantId: isolatedTenant,
				eventType: "E2EProbe",
				aggregateId: `article:${uuid()}`,
				dedupeKey,
				occurredAt,
			});

			const unrelatedUser = uuid();
			const body = await expectJsonStatus(
				await rpc.post(procedure("ListKnowledgeEvents"), {
					data: { afterSeq: "0", limit: 10, tenantId: isolatedTenant, userId: unrelatedUser },
				}),
				200,
				listEventsSchema,
			);
			expect(body.events?.map((e) => e.dedupeKey)).toContain(dedupeKey);
			// And it really is user-less on the wire, not defaulted to the
			// caller: protojson omits the empty string.
			expect(body.events?.find((e) => e.dedupeKey === dedupeKey)?.userId).toBeUndefined();
		},
	);
});

test.describe("user events", () => {
	test(
		"AppendKnowledgeUserEvent accepts a well-formed event",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage: `knowledge_user_events` is a second append-only
			// table with its own partitioning (migration 00007) and the Hurl
			// suite never wrote to it at all.
			const { occurredAt } = instant();
			const response = await rpc.post(procedure("AppendKnowledgeUserEvent"), {
				data: {
					event: {
						userEventId: uuid(),
						occurredAt,
						userId: principal.userId,
						tenantId: principal.tenantId,
						eventType: "HomeItemOpened",
						itemKey: `article:${uuid()}`,
						dedupeKey: `${principal.token}-user-event`,
						payload: encodePayload({ source: "playwright" }),
					},
				},
			});
			expect(response.status()).toBe(200);
			// The response message has no fields, so a success is literally
			// `{}` — which is also what a handler returning early would send.
			// The assertion that distinguishes them is the dedupe negative
			// below, which proves the handler actually inspects the request.
			expect(await response.json()).toEqual({});
		},
	);

	test(
		"AppendKnowledgeUserEvent refuses an empty dedupe_key",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage for a guard whose comment names its own failure
			// mode: the driver's uniqueness index is partial
			// (`WHERE dedupe_key != ''`), so an empty value does not collide —
			// it silently opts the row out of deduplication entirely. A
			// redelivering consumer would then double-count every act. The
			// handler rejects it instead, and this is the only place that can
			// observe the difference.
			const { occurredAt } = instant();
			await expectUnaryError(
				rpc,
				procedure("AppendKnowledgeUserEvent"),
				{
					event: {
						userEventId: uuid(),
						occurredAt,
						userId: principal.userId,
						tenantId: principal.tenantId,
						eventType: "HomeItemOpened",
						itemKey: `article:${uuid()}`,
						dedupeKey: "   ",
					},
				},
				"invalid_argument",
			);
		},
	);
});

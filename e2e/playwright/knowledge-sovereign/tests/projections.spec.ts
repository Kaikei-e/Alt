import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { expectUnaryError } from "../../_shared/connect.js";
import { eventually, eventuallyValue } from "../../_shared/eventual.js";
import { uuid } from "../../_shared/ids.js";
import { procedure } from "../src/env.js";
import { appendEvent, instant } from "../src/fixtures.js";
import {
	homeItemsPopulatedSchema,
	homeItemsSchema,
	projectionCheckpointSchema,
	recallCandidatesPopulatedSchema,
	todayDigestFoundSchema,
	trailFootprintsSchema,
} from "../src/schemas.js";

/**
 * The Knowledge Home / Trail read models, and the in-process projectors that
 * build them.
 *
 * Ports `06-home-items-empty.hurl`, `07-today-digest-empty.hurl` and
 * `08-recall-candidates-empty.hurl` — and then goes considerably further,
 * because those three scenarios were the weakest in the retired suite.
 *
 * All three asserted that a projection was **empty**, and their comments
 * explained why: "Phase 1 scope validates the structure contract without
 * requiring projector wiring", "projector wiring is out of scope". That was
 * true when they were written; it is not true now. `main.go` starts
 * `knowledge_home_projector` and `knowledge_trail_projector` in-process on a
 * ticker (2s in the staging slice), so this service folds its own event log.
 *
 * Which means the Hurl assertions had quietly become the wrong shape: they ran
 * `AppendKnowledgeEvent` in scenario 03 and then asserted the *same user's*
 * home projection was empty in scenario 06. They passed for a reason nobody
 * intended — the payload they sent, `{"title":"hurl-e2e"}`, has no
 * `article_id`, so `foldArticleCreated`'s `uuid.Parse` fails, the batch stops
 * without advancing, and the projector jams on that event for the lifetime of
 * the stack. A green suite was recording a wedged projector.
 *
 * So the empty-projection assertions are kept, but moved onto users that
 * genuinely have no events — and the positive direction, which is the one
 * CLAUDE.md rule 8 cares about, is added: an unwired projector is
 * indistinguishable from an idle one unless something asserts convergence.
 */

/** A well-formed `ArticleCreated` payload — one the projector can actually fold. */
function articleCreatedPayload(articleId: string, title: string) {
	return {
		article_id: articleId,
		title,
		url: `https://example.invalid/${articleId}`,
		published_at: instant(-60 * 60 * 1000).occurredAt,
	};
}

test.describe("a principal with no events has empty projections", () => {
	test(
		"GetKnowledgeHomeItems returns nothing",
		{ tag: "@smoke" },
		async ({ rpc, principal }) => {
			// `06-home-items-empty.hurl` asserted `$.items not exists`,
			// `$.hasMore not exists` and `$.nextCursor not exists` as three
			// separate checks. Comparing the whole document is strictly
			// stronger: it also fails if the handler grows a fourth field, or
			// starts answering `{"items": null}` — which is what a Go handler
			// does the moment someone drops the `make([]*T, len(items))` and
			// returns a nil slice through a codec configured to emit it.
			const response = await rpc.post(procedure("GetKnowledgeHomeItems"), {
				data: { userId: principal.userId, limit: 10 },
			});
			expect(response.status()).toBe(200);
			expect(await response.json()).toEqual({});
		},
	);

	test("GetTodayDigest returns no digest", { tag: "@smoke" }, async ({ rpc, principal }) => {
		// Port of `07-today-digest-empty.hurl`. The handler returns a nil
		// `*TodayDigest` when the row is missing, and protojson omits a nil
		// message — so "no digest today" is `{}`, never a 404 and never a
		// zero-valued digest. A caller that switched on `digest == null` and a
		// handler that started emitting `{"digest":{...zeros...}}` would
		// disagree about whether the user has read anything today.
		const response = await rpc.post(procedure("GetTodayDigest"), {
			data: { userId: principal.userId, date: instant().date },
		});
		expect(response.status()).toBe(200);
		expect(await response.json()).toEqual({});
	});

	test("GetRecallCandidates returns nothing", { tag: "@smoke" }, async ({ rpc, principal }) => {
		// Port of `08-recall-candidates-empty.hurl`.
		const response = await rpc.post(procedure("GetRecallCandidates"), {
			data: { userId: principal.userId, limit: 10 },
		});
		expect(response.status()).toBe(200);
		expect(await response.json()).toEqual({});
	});

	test(
		"GetTrailFootprints returns an empty spine",
		{ tag: "@smoke" },
		async ({ rpc, principal }) => {
			// New coverage: the Trail spine (migrations 00026/00027) did not
			// exist when the Hurl suite was written and was never added to it.
			const body = await expectJsonStatus(
				await rpc.post(procedure("GetTrailFootprints"), {
					data: { userId: principal.userId, limit: 20 },
				}),
				200,
				trailFootprintsSchema,
			);
			expect(body.episodes ?? []).toEqual([]);
			expect(body.branches ?? []).toEqual([]);
		},
	);
});

test.describe("the in-process projectors fold the event log", () => {
	test(
		"ArticleCreated becomes a Knowledge Home item",
		{ tag: "@slow" },
		async ({ rpc, principal }) => {
			// New coverage, and the assertion this whole file exists for.
			//
			// `main.go` logs `home.projector.wiring enabled=true` at startup
			// (CLAUDE.md rule 8) but nothing downstream *checks* it. Delete the
			// ticker goroutine and every unit test still passes, `/health`
			// still answers ok, and the Knowledge Home is simply empty forever
			// — the exact silent-degradation shape PM-2026-045 / ADR-000928
			// were written about. Convergence of a seeded event is the only
			// externally observable proof the fold is running.
			//
			// `eventually` rather than a sleep: the tick is 2s in staging and
			// 5s by default, so a fixed wait is either flaky or wasteful.
			test.setTimeout(90_000);
			const articleId = uuid();
			const { occurredAt } = instant();
			await appendEvent(rpc, {
				tenantId: principal.tenantId,
				userId: principal.userId,
				eventType: "ArticleCreated",
				aggregateType: "article",
				aggregateId: `article:${articleId}`,
				dedupeKey: `${principal.token}-article-created`,
				occurredAt,
				payload: articleCreatedPayload(articleId, `playwright ${principal.token}`),
			});

			await eventually(
				async () => {
					const body = await expectJsonStatus(
						await rpc.post(procedure("GetKnowledgeHomeItems"), {
							data: { userId: principal.userId, limit: 10 },
						}),
						200,
						homeItemsPopulatedSchema,
					);
					const item = body.items.find((i) => i.itemKey === `article:${articleId}`);
					expect(item, `no home item for article:${articleId}`).toBeDefined();
					// Every field below is derived from the event payload
					// alone — never from a lookup against another read model.
					// That is the reproject-safety invariant, and asserting the
					// values (not just the row's existence) is what would catch
					// a fold that started reading the title from somewhere else.
					expect(item?.title).toBe(`playwright ${principal.token}`);
					expect(item?.url).toBe(`https://example.invalid/${articleId}`);
					expect(item?.itemType).toBe("article");
					expect(item?.primaryRefId).toBe(articleId);
					expect(item?.summaryState).toBe("pending");
					expect(item?.whyReasons?.map((w) => w.code)).toContain("new_unread");
					// The row must be stamped with the version the read path
					// filters on, or it is written and immediately invisible.
					expect(item?.projectionVersion).toBe(1);
				},
				{
					timeout: 60_000,
					message: "knowledge_home_projector folds ArticleCreated into knowledge_home_items",
				},
			);
		},
	);

	test(
		"ArticleCreated also folds today_digest_view",
		{ tag: "@slow" },
		async ({ rpc, principal }) => {
			// New coverage. The digest upsert is deliberately **non-fatal** in
			// `foldArticleCreated` — a failure is logged at WARN and the batch
			// continues — so a broken digest write is invisible in every
			// signal except this one. That is what makes it worth a test of
			// its own rather than an extra assertion on the item test: the two
			// fail independently by design.
			test.setTimeout(90_000);
			const articleId = uuid();
			const stamp = instant();
			await appendEvent(rpc, {
				tenantId: principal.tenantId,
				userId: principal.userId,
				eventType: "ArticleCreated",
				aggregateType: "article",
				aggregateId: `article:${articleId}`,
				dedupeKey: `${principal.token}-digest`,
				occurredAt: stamp.occurredAt,
				payload: articleCreatedPayload(articleId, `digest ${principal.token}`),
			});

			await eventually(
				async () => {
					const body = await expectJsonStatus(
						await rpc.post(procedure("GetTodayDigest"), {
							// The date is derived from the same instant as
							// occurred_at, because the projector keys the row on
							// `occurredAt.Format(time.DateOnly)`. Two separate
							// clock reads disagree for a few milliseconds a day,
							// at midnight UTC.
							data: { userId: principal.userId, date: stamp.date },
						}),
						200,
						todayDigestFoundSchema,
					);
					expect(body.digest.digestDate).toBe(stamp.date);
					expect(body.digest.newArticles).toBe(1);
					// The article carries no summary yet, so it counts as
					// unsummarized — the counter the "N to catch up on" figure
					// in the SPA is built from.
					expect(body.digest.unsummarizedArticles).toBe(1);
				},
				{
					timeout: 60_000,
					message: "knowledge_home_projector folds ArticleCreated into today_digest_view",
				},
			);
		},
	);

	test(
		"the home projector checkpoint advances past a newly appended event",
		{ tag: "@slow" },
		async ({ rpc, principal }) => {
			// New coverage, aimed at a different failure than the two above: a
			// projector that is *running but stuck*. `RunBatch` stops without
			// advancing past an event whose fold returns an error, and retries
			// it every tick forever — which is exactly what the retired Hurl
			// suite left the stack in. From the outside a wedged projector and
			// a healthy idle one look identical; the checkpoint is the only
			// place the difference is visible.
			test.setTimeout(90_000);
			const { occurredAt } = instant();
			const seq = await appendEvent(rpc, {
				tenantId: principal.tenantId,
				userId: principal.userId,
				// An event type no fold handles: `foldEvent`'s default branch
				// skips it and the caller still advances the checkpoint. That
				// is precisely the behaviour under test — an unknown type must
				// not wedge the log.
				eventType: "E2ECheckpointProbe",
				aggregateId: `article:${uuid()}`,
				dedupeKey: `${principal.token}-checkpoint`,
				occurredAt,
			});

			await eventuallyValue(
				async () => {
					const body = await expectJsonStatus(
						await rpc.post(procedure("GetProjectionCheckpoint"), {
							data: { projectorName: "knowledge-home-projector" },
						}),
						200,
						projectionCheckpointSchema,
					);
					return Number(body.lastEventSeq ?? "0");
				},
				`knowledge-home-projector's checkpoint reaches event_seq ${seq}`,
				{ timeout: 60_000 },
			).toBeGreaterThanOrEqual(Number(seq));
		},
	);

	test(
		"HomeItemOpened becomes a read footprint on the Trail spine",
		{ tag: "@slow" },
		async ({ rpc, principal }) => {
			// New coverage for a second projector with the same wiring hazard.
			// `verbByEventType` maps HomeItemOpened → "read", and
			// `footprintFromEvent` refuses to emit unless the event carries
			// both a user_id and an aggregate_id — so this also pins that the
			// footprint's `item_key` comes from `aggregate_id`, not from the
			// payload.
			test.setTimeout(90_000);
			const itemKey = `article:${uuid()}`;
			const { occurredAt } = instant();
			await appendEvent(rpc, {
				tenantId: principal.tenantId,
				userId: principal.userId,
				eventType: "HomeItemOpened",
				aggregateType: "article",
				aggregateId: itemKey,
				dedupeKey: `${principal.token}-opened`,
				occurredAt,
				payload: { item_key: itemKey },
			});

			await eventually(
				async () => {
					const body = await expectJsonStatus(
						await rpc.post(procedure("GetTrailFootprints"), {
							data: { userId: principal.userId, limit: 20 },
						}),
						200,
						trailFootprintsSchema,
					);
					const footprints = (body.episodes ?? []).flatMap((e) => e.footprints);
					const footprint = footprints.find((f) => f.itemKey === itemKey);
					expect(footprint, `no trail footprint for ${itemKey}`).toBeDefined();
					expect(footprint?.verb).toBe("read");
					expect(footprint?.sourceEventType).toBe("HomeItemOpened");
					// contactCount is derived, and `mapTrailFootprints` floors
					// it at 1 for legacy single-contact rows — so a 0 here
					// would mean the floor was removed and the SPA would render
					// "0 visits".
					expect(footprint?.contactCount).toBeGreaterThanOrEqual(1);
					expect(footprint?.userId).toBe(principal.userId);
					expect(footprint?.tenantId).toBe(principal.tenantId);
				},
				{
					timeout: 60_000,
					message: "knowledge_trail_projector folds HomeItemOpened into a read footprint",
				},
			);
		},
	);

	test(
		"an opened item becomes an eligible recall candidate once its cooldown has passed",
		{ tag: "@slow" },
		async ({ rpc, principal }) => {
			// New coverage for the branch of `foldHomeItemOpened` that writes
			// `recall_candidate_view`, and for the read filter that gates it.
			//
			// The fold sets `first_eligible_at = occurred_at + 1h`, and
			// `GetRecallCandidates` filters `next_suggest_at <= now()`. An
			// event stamped *now* therefore projects a candidate that is
			// correctly invisible, which is why the empty-projection test above
			// stays true even for users who have opened something. Backdating
			// the event by two hours is what makes the row eligible — and it
			// works only because the fold derives eligibility from the event's
			// own `occurred_at` rather than from wall clock. If someone
			// replaced that with `time.Now()`, reproject would stop being
			// deterministic and this test would fail.
			test.setTimeout(90_000);
			const itemKey = `article:${uuid()}`;
			const twoHoursAgo = instant(-2 * 60 * 60 * 1000);
			await appendEvent(rpc, {
				tenantId: principal.tenantId,
				userId: principal.userId,
				eventType: "HomeItemOpened",
				aggregateType: "article",
				aggregateId: itemKey,
				dedupeKey: `${principal.token}-recall`,
				occurredAt: twoHoursAgo.occurredAt,
				payload: { item_key: itemKey },
			});

			await eventually(
				async () => {
					const body = await expectJsonStatus(
						await rpc.post(procedure("GetRecallCandidates"), {
							data: { userId: principal.userId, limit: 10 },
						}),
						200,
						recallCandidatesPopulatedSchema,
					);
					const candidate = body.candidates.find((c) => c.itemKey === itemKey);
					expect(candidate, `no recall candidate for ${itemKey}`).toBeDefined();
					// The "why" is first-class (immutable data model): a
					// candidate that cannot explain itself must not surface.
					expect(candidate?.reasons?.map((r) => r.type)).toContain(
						"opened_before_but_not_revisited",
					);
					expect(candidate?.projectionVersion).toBe(1);
				},
				{
					timeout: 60_000,
					message:
						"knowledge_home_projector folds HomeItemOpened into an eligible recall_candidate_view row",
				},
			);
		},
	);
});

test.describe("projection read validation", () => {
	test("GetTodayDigest rejects a malformed date", { tag: "@contract" }, async ({
		rpc,
		principal,
	}) => {
		// New coverage. `parseDateField` returns InvalidArgument rather than
		// silently coercing to "today", and the comment in read_operations.go
		// says why: a digest silently answered for the wrong day is a wrong
		// answer that looks right, and the caller never learns its date was
		// discarded.
		const error = await expectUnaryError(
			rpc,
			procedure("GetTodayDigest"),
			{ userId: principal.userId, date: "2026-13-45" },
			"invalid_argument",
		);
		expect(error.message ?? "").toContain("date");
	});

	test("GetTodayDigest treats an absent date as today", { tag: "@contract" }, async ({
		rpc,
		principal,
	}) => {
		// The other half of the same guard: an *empty* date is "absent", which
		// the handler explicitly defaults to `time.Now()`. Only an unparseable
		// value is an error. Without this test the negative above could be
		// satisfied by a handler that rejected every request.
		const response = await rpc.post(procedure("GetTodayDigest"), {
			data: { userId: principal.userId },
		});
		expect(response.status()).toBe(200);
		expect(await response.json()).toEqual({});
	});

	test("GetTrailFootprints rejects a malformed cursor", { tag: "@contract" }, async ({
		rpc,
		principal,
	}) => {
		// New coverage. `parseEpisodeCursor` accepts only "" or "ep:<n>" and
		// rejects anything else "rather than silently reset to page 1". That
		// distinction is the difference between a client noticing it sent a
		// stale cursor and a client paging the same first screen forever.
		await expectUnaryError(
			rpc,
			procedure("GetTrailFootprints"),
			{ userId: principal.userId, limit: 20, cursor: "not-an-episode-cursor" },
			"invalid_argument",
		);
		await expectUnaryError(
			rpc,
			procedure("GetTrailFootprints"),
			{ userId: principal.userId, limit: 20, cursor: "ep:-3" },
			"invalid_argument",
		);
	});

	test("GetTrailBranchesForAnchor requires an anchor", { tag: "@contract" }, async ({
		rpc,
		principal,
	}) => {
		// New coverage. The patch-exit surface (D26) is anchored on one item by
		// definition; a blank anchor would silently widen it to the user's
		// whole branch inbox, which is the surface that was deliberately
		// retired. The handler trims and rejects.
		await expectUnaryError(
			rpc,
			procedure("GetTrailBranchesForAnchor"),
			{ userId: principal.userId, anchorItemKey: "   ", limit: 5 },
			"invalid_argument",
		);
	});

	test(
		"GetKnowledgeHomeItems accepts a lens filter without inventing rows",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage for the `filter` sub-message, which the Hurl suite
			// never populated. Passing one must not change the answer for a
			// user with no rows — a filter that widened rather than narrowed
			// (an `OR` where an `AND` belongs, the classic) would show up here
			// as somebody else's items.
			const body = await expectJsonStatus(
				await rpc.post(procedure("GetKnowledgeHomeItems"), {
					data: {
						userId: principal.userId,
						limit: 10,
						filter: {
							queryText: "playwright",
							tagIds: ["nonexistent-tag"],
							timeWindow: "7d",
							includeRecap: true,
							includePulse: true,
							sortMode: "relevance",
						},
					},
				}),
				200,
				homeItemsSchema,
			);
			expect(body.items ?? []).toEqual([]);
		},
	);
});

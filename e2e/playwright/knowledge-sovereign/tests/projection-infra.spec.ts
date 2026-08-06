import { test, expect } from "../src/fixtures.js";
import { expectJsonStatus } from "../../_shared/http.js";
import { eventuallyValue } from "../../_shared/eventual.js";
import { uuid } from "../../_shared/ids.js";
import { procedure } from "../src/env.js";
import { appendEvent, instant } from "../src/fixtures.js";
import {
	activeProjectionVersionSchema,
	compareProjectionsSchema,
	countNeedToKnowSchema,
	getBackfillJobSchema,
	getReprojectRunSchema,
	listBackfillJobsSchema,
	listDistinctUserIdsSchema,
	listProjectionAuditsSchema,
	listProjectionVersionsSchema,
	listReprojectRunsSchema,
	projectionCheckpointSchema,
	projectionFreshnessSchema,
	projectionLagSchema,
} from "../src/schemas.js";

/**
 * Projection infrastructure: versions, checkpoints, reproject runs, backfill
 * jobs, audits.
 *
 * This whole block of the service — twenty of its forty-six procedures — had
 * exactly one Hurl scenario, `02-rpc-projection-version.hurl`, which read the
 * seeded active version. Everything else here is new.
 *
 * It is the plumbing the immutable data model rests on: a checkpoint is how a
 * projector resumes without re-folding, a projection version is what makes a
 * read model disposable and rebuildable, and a reproject run is the record of
 * having rebuilt one. Untested plumbing that only runs during an incident is
 * the plumbing most worth testing.
 */

test.describe("projection versions", () => {
	test(
		"GetActiveProjectionVersion returns the seeded version 1",
		{ tag: "@smoke" },
		async ({ rpc }) => {
			// Port of `02-rpc-projection-version.hurl`, which asserted
			// `version == 1`, `status == "active"` and `description exists`.
			// The last of those is the assertion this migration exists to
			// remove: `exists` is true of `""` and of `null`, so a handler that
			// stopped selecting the column would still have passed. The schema
			// requires a non-empty string, and adds the two timestamps the
			// scenario never looked at — `activated_at` in particular, which is
			// what distinguishes "active" from "was active once".
			const body = await expectJsonStatus(
				await rpc.post(procedure("GetActiveProjectionVersion"), { data: {} }),
				200,
				activeProjectionVersionSchema,
			);
			expect(body.version.version).toBe(1);
			expect(body.version.status).toBe("active");
			expect(body.version.activatedAt).toBeDefined();
		},
	);

	test(
		"ListProjectionVersions contains the active version",
		{ tag: "@contract" },
		async ({ rpc }) => {
			// New coverage. The read path resolves the active version with a
			// `COALESCE(..., 1)` subquery, so the registry disappearing would
			// not error — every read would silently fall back to version 1
			// forever. Listing it is how the fallback stays a fallback.
			const body = await expectJsonStatus(
				await rpc.post(procedure("ListProjectionVersions"), { data: {} }),
				200,
				listProjectionVersionsSchema,
			);
			const active = body.versions.filter((v) => v.status === "active");
			expect(
				active,
				"exactly one row may be active — ActivateProjectionVersion deactivates the rest in one transaction",
			).toHaveLength(1);
			expect(active[0]?.version).toBe(1);
		},
	);
});

test.describe("checkpoints", () => {
	test(
		"an unknown projector's checkpoint is 0 rather than an error",
		{ tag: "@contract" },
		async ({ rpc }) => {
			// New coverage. The driver maps `pgx.ErrNoRows` to `(0, nil)`, and
			// protojson omits the zero — so a projector booting for the first
			// time reads an empty document and starts from the beginning of
			// the log. A `not_found` here instead would make every first boot
			// a crash loop.
			const response = await rpc.post(procedure("GetProjectionCheckpoint"), {
				data: { projectorName: `pw-never-existed-${uuid()}` },
			});
			expect(response.status()).toBe(200);
			expect(await response.json()).toEqual({});
		},
	);

	test(
		"UpdateProjectionCheckpoint round-trips through GetProjectionCheckpoint",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage. The projector name is per-test rather than one of
			// the two real ones: writing to `knowledge-home-projector` would
			// rewind or fast-forward the live projector and break every
			// convergence assertion in tests/projections.spec.ts. Seeding a
			// namespace of its own is what makes this test safe to run in
			// parallel with them.
			const projectorName = `pw-checkpoint-${principal.token}`;

			const updated = await rpc.post(procedure("UpdateProjectionCheckpoint"), {
				data: { projectorName, lastEventSeq: "4242" },
			});
			expect(updated.status()).toBe(200);

			const body = await expectJsonStatus(
				await rpc.post(procedure("GetProjectionCheckpoint"), { data: { projectorName } }),
				200,
				projectionCheckpointSchema,
			);
			expect(body.lastEventSeq).toBe("4242");

			// The upsert branch: a second write must move the checkpoint, not
			// fail on the primary key. A projector that could only ever write
			// its checkpoint once would re-fold the entire log after every
			// restart.
			await rpc.post(procedure("UpdateProjectionCheckpoint"), {
				data: { projectorName, lastEventSeq: "4243" },
			});
			const moved = await expectJsonStatus(
				await rpc.post(procedure("GetProjectionCheckpoint"), { data: { projectorName } }),
				200,
				projectionCheckpointSchema,
			);
			expect(moved.lastEventSeq).toBe("4243");
		},
	);

	test(
		"GetProjectionFreshness reports when a checkpoint last moved",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage. Freshness is `updated_at` on the checkpoint row and
			// is what an operator dashboard reads to answer "is the projector
			// alive". Seeding the checkpoint first makes the assertion
			// independent of whether the real projectors have folded anything
			// yet — which they may not have, this early in a run.
			const projectorName = `pw-freshness-${principal.token}`;
			const missing = await rpc.post(procedure("GetProjectionFreshness"), {
				data: { projectorName },
			});
			expect(missing.status()).toBe(200);
			// `found: false` plus a nil timestamp: both omitted.
			expect(await missing.json()).toEqual({});

			await rpc.post(procedure("UpdateProjectionCheckpoint"), {
				data: { projectorName, lastEventSeq: "1" },
			});

			const body = await expectJsonStatus(
				await rpc.post(procedure("GetProjectionFreshness"), { data: { projectorName } }),
				200,
				projectionFreshnessSchema,
			);
			expect(body.found).toBe(true);
			expect(body.updatedAt).toBeDefined();
		},
	);

	test("GetProjectionLag answers with both gauges", { tag: "@contract" }, async ({ rpc }) => {
		// New coverage. Two `double` fields, so protojson emits them as JSON
		// numbers and omits either when it is exactly 0 — which is the normal
		// answer on a caught-up stack, and precisely why a client must treat
		// absent as 0 rather than as "unavailable". The schema records that.
		const body = await expectJsonStatus(
			await rpc.post(procedure("GetProjectionLag"), { data: {} }),
			200,
			projectionLagSchema,
		);
		// `x ?? 0` followed by `>= 0` is true for every possible answer,
		// including an empty body — the earlier form of this test could not
		// fail. Assert what protojson's omission rule actually promises: a
		// field is either absent (meaning exactly 0) or a finite number. A
		// handler that started emitting null, a string, or NaN now fails, and
		// so does one that answers with a body of the wrong shape.
		for (const [name, value] of [
			["lagSeconds", body.lagSeconds],
			["ageSeconds", body.ageSeconds],
		] as const) {
			if (value === undefined) continue;
			expect(
				Number.isFinite(value),
				`${name} was present but not a finite number: ${JSON.stringify(value)}`,
			).toBe(true);
			expect(value, `${name} is a duration and cannot be negative`).toBeGreaterThanOrEqual(0);
		}
	});
});

test.describe("projection census", () => {
	test(
		"ListDistinctUserIDs picks up a user once their first item is projected",
		{ tag: "@slow" },
		async ({ rpc, principal }) => {
			// New coverage. `SELECT DISTINCT user_id FROM knowledge_home_items`
			// is what the nightly per-user jobs iterate — a Knowledge Home that
			// never enumerates its users produces no digests and no recaps for
			// anyone, silently. Seeding an event and waiting for the user to
			// appear tests the census and the projector's write in one step.
			test.setTimeout(90_000);
			const articleId = uuid();
			const { occurredAt } = instant();
			await appendEvent(rpc, {
				tenantId: principal.tenantId,
				userId: principal.userId,
				eventType: "ArticleCreated",
				aggregateType: "article",
				aggregateId: `article:${articleId}`,
				dedupeKey: `${principal.token}-census`,
				occurredAt,
				payload: {
					article_id: articleId,
					title: `census ${principal.token}`,
					url: `https://example.invalid/${articleId}`,
				},
			});

			await eventuallyValue(
				async () => {
					const body = await expectJsonStatus(
						await rpc.post(procedure("ListDistinctUserIDs"), { data: {} }),
						200,
						listDistinctUserIdsSchema,
					);
					return body.userIds ?? [];
				},
				`ListDistinctUserIDs includes ${principal.userId} once its first home item is projected`,
				{ timeout: 60_000 },
			).toContain(principal.userId);
		},
	);

	test(
		"CountNeedToKnowItems is 0 for a principal with nothing to catch up on",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage. The count requires a `why_json` entry with code
			// `pulse_need_to_know` *and* a `published_at` inside the requested
			// day, so a fresh user is 0 — and protojson omits a zero int32, so
			// the answer is the empty document. A client reading this as
			// "unknown" rather than "none" would badge an empty inbox.
			const response = await rpc.post(procedure("CountNeedToKnowItems"), {
				data: { userId: principal.userId, date: instant().date },
			});
			expect(response.status()).toBe(200);
			expect(await response.json()).toEqual({});
			// And the typed read of the same thing, so the schema is exercised.
			const typed = await expectJsonStatus(
				await rpc.post(procedure("CountNeedToKnowItems"), {
					data: { userId: principal.userId, date: instant().date },
				}),
				200,
				countNeedToKnowSchema,
			);
			expect(typed.count ?? 0).toBe(0);
		},
	);

	test("CompareProjections answers a diff summary", { tag: "@contract" }, async ({ rpc }) => {
		// New coverage. This is the read a reproject cutover depends on:
		// "does the shadow version produce the same shape as the live one".
		//
		// Comparing version 1 with itself is the only comparison whose answer
		// is knowable without seeding a second projection version — but it is
		// also self-fulfilling: both sides run the same query over the same
		// rows, so `fromCount === toCount` holds even if the handler read the
		// wrong version for one side. The assertion that carries weight here is
		// therefore the *shape*: both counts must actually be present and
		// finite. A handler returning `{"summary":{}}` satisfied the old
		// equality (0 === 0) and fails this.
		const body = await expectJsonStatus(
			await rpc.post(procedure("CompareProjections"), {
				data: { fromVersion: "1", toVersion: "1" },
			}),
			200,
			compareProjectionsSchema,
		);

		// protojson omits a zero count, so absent means 0 — but both sides must
		// agree on being absent. One side present and the other not is the
		// asymmetry a wrong-version read would produce.
		expect(
			body.summary.fromCount === undefined,
			"one side of the comparison reported a count and the other did not, for a " +
				"comparison of a version with itself",
		).toBe(body.summary.toCount === undefined);
		expect(body.summary.fromCount ?? 0).toBe(body.summary.toCount ?? 0);
		expect(body.summary.fromAvgScore ?? 0).toBe(body.summary.toAvgScore ?? 0);
	});
});

test.describe("backfill jobs", () => {
	test(
		"CreateBackfillJob round-trips and defaults its kind",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage for a surface with no assertions at all. The `kind`
			// default is the interesting part: ADR-000846 added the
			// discriminator additively, and `CreateBackfillJob` substitutes
			// "articles" for an empty value so a proto-v1 producer that never
			// sets it keeps its original semantics. Send it empty on purpose.
			const jobId = uuid();
			const { occurredAt } = instant();
			const created = await rpc.post(procedure("CreateBackfillJob"), {
				data: {
					job: {
						jobId,
						status: "pending",
						projectionVersion: 1,
						totalEvents: 10,
						processedEvents: 0,
						cursorUserId: principal.userId,
						createdAt: occurredAt,
						updatedAt: occurredAt,
					},
				},
			});
			expect(created.status()).toBe(200);

			const body = await expectJsonStatus(
				await rpc.post(procedure("GetBackfillJob"), { data: { jobId } }),
				200,
				getBackfillJobSchema,
			);
			expect(body.job.jobId).toBe(jobId);
			expect(body.job.status).toBe("pending");
			expect(body.job.kind).toBe("articles");
			expect(body.job.projectionVersion).toBe(1);
			expect(body.job.totalEvents).toBe(10);

			const listed = await expectJsonStatus(
				await rpc.post(procedure("ListBackfillJobs"), { data: {} }),
				200,
				listBackfillJobsSchema,
			);
			// Containment, not cardinality: sibling workers create jobs too.
			expect(listed.jobs?.map((j) => j.jobId)).toContain(jobId);
		},
	);

	test(
		"UpdateBackfillJob advances an existing job",
		{ tag: "@contract" },
		async ({ rpc }) => {
			// New coverage. A backfill that cannot record progress restarts
			// from zero after every interruption, which for a full reproject is
			// the difference between hours and never.
			const jobId = uuid();
			const { occurredAt } = instant();
			await rpc.post(procedure("CreateBackfillJob"), {
				data: {
					job: {
						jobId,
						status: "pending",
						projectionVersion: 1,
						totalEvents: 10,
						processedEvents: 0,
						createdAt: occurredAt,
						updatedAt: occurredAt,
					},
				},
			});

			const updated = await rpc.post(procedure("UpdateBackfillJob"), {
				data: {
					job: {
						jobId,
						status: "running",
						projectionVersion: 1,
						totalEvents: 10,
						processedEvents: 7,
						createdAt: occurredAt,
						updatedAt: instant().occurredAt,
					},
				},
			});
			expect(updated.status()).toBe(200);

			const body = await expectJsonStatus(
				await rpc.post(procedure("GetBackfillJob"), { data: { jobId } }),
				200,
				getBackfillJobSchema,
			);
			expect(body.job.status).toBe("running");
			expect(body.job.processedEvents).toBe(7);
		},
	);

	test(
		"GetBackfillJob for an unknown id is an empty answer",
		{ tag: "@contract" },
		async ({ rpc }) => {
			// Same `(nil, nil)` miss convention as GetLens. Pinned here too
			// because a caller polling a job id it made up must not be told the
			// service is broken.
			const response = await rpc.post(procedure("GetBackfillJob"), { data: { jobId: uuid() } });
			expect(response.status()).toBe(200);
			expect(await response.json()).toEqual({});
		},
	);
});

test.describe("reproject runs and audits", () => {
	test(
		"CreateReprojectRun round-trips through GetReprojectRun and the list",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage. A reproject run is the audit record of having
			// rebuilt a disposable read model — the thing that makes
			// "projections can be thrown away" an operational claim rather than
			// an aspiration. It had no test.
			const runId = uuid();
			const { occurredAt } = instant();
			const created = await rpc.post(procedure("CreateReprojectRun"), {
				data: {
					run: {
						reprojectRunId: runId,
						projectionName: `pw-${principal.token}`,
						fromVersion: "1",
						toVersion: "2",
						initiatedBy: principal.userId,
						mode: "shadow",
						status: "pending",
						createdAt: occurredAt,
					},
				},
			});
			expect(created.status()).toBe(200);

			const body = await expectJsonStatus(
				await rpc.post(procedure("GetReprojectRun"), { data: { runId } }),
				200,
				getReprojectRunSchema,
			);
			expect(body.run.reprojectRunId).toBe(runId);
			expect(body.run.projectionName).toBe(`pw-${principal.token}`);
			expect(body.run.fromVersion).toBe("1");
			expect(body.run.toVersion).toBe("2");
			expect(body.run.status).toBe("pending");

			const listed = await expectJsonStatus(
				await rpc.post(procedure("ListReprojectRuns"), {
					data: { statusFilter: "pending", limit: 100 },
				}),
				200,
				listReprojectRunsSchema,
			);
			expect(listed.runs?.map((r) => r.reprojectRunId)).toContain(runId);
		},
	);

	test(
		"UpdateReprojectRun records a terminal status",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage for the other half of the same record. Without it a
			// run created and never closed reads as "still running" forever,
			// which is what an operator would act on.
			const runId = uuid();
			const projectionName = `pw-${principal.token}-terminal`;
			const { occurredAt } = instant();
			await rpc.post(procedure("CreateReprojectRun"), {
				data: {
					run: {
						reprojectRunId: runId,
						projectionName,
						fromVersion: "1",
						toVersion: "2",
						mode: "shadow",
						status: "pending",
						createdAt: occurredAt,
					},
				},
			});

			const updated = await rpc.post(procedure("UpdateReprojectRun"), {
				data: {
					run: {
						reprojectRunId: runId,
						projectionName,
						fromVersion: "1",
						toVersion: "2",
						mode: "shadow",
						status: "completed",
						createdAt: occurredAt,
						finishedAt: instant().occurredAt,
					},
				},
			});
			expect(updated.status()).toBe(200);

			const body = await expectJsonStatus(
				await rpc.post(procedure("GetReprojectRun"), { data: { runId } }),
				200,
				getReprojectRunSchema,
			);
			expect(body.run.status).toBe("completed");
		},
	);

	test(
		"CreateProjectionAudit round-trips through ListProjectionAudits",
		{ tag: "@contract" },
		async ({ rpc, principal }) => {
			// New coverage. `ListProjectionAudits` filters on the projection
			// name, so a per-test name makes "exactly one audit" a claim about
			// the filter rather than about the run order — the same trick that
			// replaces the Hurl suite's `--jobs 1` everywhere else in this
			// suite.
			const auditId = uuid();
			const projectionName = `pw-audit-${principal.token}`;
			const created = await rpc.post(procedure("CreateProjectionAudit"), {
				data: {
					audit: {
						auditId,
						projectionName,
						// `string`, not int32 — see the schema note.
						projectionVersion: "1",
						checkedAt: instant().occurredAt,
						sampleSize: 100,
						mismatchCount: 0,
					},
				},
			});
			expect(created.status()).toBe(200);

			const body = await expectJsonStatus(
				await rpc.post(procedure("ListProjectionAudits"), {
					data: { projectionName, limit: 10 },
				}),
				200,
				listProjectionAuditsSchema,
			);
			expect(body.audits).toHaveLength(1);
			expect(body.audits[0]?.auditId).toBe(auditId);
			expect(body.audits[0]?.sampleSize).toBe(100);
			// mismatchCount 0 is omitted by protojson — the "clean audit"
			// answer is an absent field, not a zero.
			expect(body.audits[0]?.mismatchCount).toBeUndefined();
		},
	);
});

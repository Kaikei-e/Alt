import { z } from "zod";

/**
 * Response envelopes, as Zod schemas.
 *
 * The Hurl suite this replaces asserted one JSONPath at a time, so a scenario
 * that "checked" a response usually checked one field of it — and four of its
 * twenty scenarios asserted only that a collection *was* a collection, which
 * `[]` satisfies forever. A schema asserts the whole envelope in one step, so
 * a handler that changes shape fails here instead of quietly passing.
 *
 * Every object schema is `passthrough()`: a contract on the fields
 * knowledge-sovereign promises, not a freeze on the ones it may add.
 *
 * ── Two wire dialects live in this file ──
 *
 * The **RPC port (:9500)** speaks proto3-JSON through connect-go's default
 * codec, which is `protojson` with `EmitUnpopulated: false`. That has three
 * consequences every schema below is written around:
 *
 *   1. `int64` is a JSON **string** (`"7"`), not a number.
 *   2. A zero/empty/false scalar, an empty repeated field and a nil message
 *      are **omitted entirely**. `hasMore: false` is not `false` on the wire,
 *      it is absent. So `.optional()` here usually means "this field's zero
 *      value", never "the server might not implement it".
 *   3. Field names are lowerCamelCase, not the proto's snake_case.
 *
 * The **operator port (:9501)** is plain `encoding/json` over Go structs with
 * explicit snake_case tags (ADR-000942, which superseded ADR-000765 §3's
 * PascalCase-no-tags stance so `altctl home snapshot list` could decode them).
 * Nothing is omitted there unless the tag says `omitempty`, so those schemas
 * demand every field.
 */

export {
	uuidSchema,
	timestampSchema,
	dateSchema,
	nonEmptyArray,
} from "../../_shared/schemas.js";
export { connectErrorSchema, ConnectCode } from "../../_shared/connect.js";

import { dateSchema, timestampSchema, uuidSchema } from "../../_shared/schemas.js";

// ---------------------------------------------------------------------------
// Primitives
// ---------------------------------------------------------------------------

/**
 * proto3-JSON `int64`.
 *
 * Asserting `z.number()` here would pass in TypeScript's mind and fail on the
 * wire: protojson emits 64-bit integers as strings because a JS number cannot
 * hold them. The Hurl suite encoded the same fact as
 * `matches "^[0-9]+$"`; this keeps it.
 */
export const int64Schema = z.string().regex(/^-?\d+$/, "proto3-JSON int64 is a decimal string");

/** A positive proto3-JSON int64 — an event_seq that actually exists. */
export const positiveInt64Schema = z
	.string()
	.regex(/^\d+$/)
	.refine((v) => v !== "0", "event_seq 0 means 'no event', not 'the first event'");

/**
 * `sha256:<64 hex>` as the snapshot handler formats it
 * (`fmt.Sprintf("sha256:%x", hasher.Sum(nil))`).
 *
 * The Hurl suite matched `^sha256:[0-9a-f]+$`, which a truncated or
 * single-nibble digest satisfies. A SHA-256 is 32 bytes; anything else is a
 * bug in `exportTable`, so the length is pinned.
 */
export const sha256Schema = z.string().regex(/^sha256:[0-9a-f]{64}$/);

// ---------------------------------------------------------------------------
// /health — both ports (handler/health.go)
// ---------------------------------------------------------------------------

/**
 * `handler.HealthHandler` writes exactly `{"status":"ok","service":"..."}`.
 *
 * The service name is asserted, not just the status: both listeners of every
 * Alt Go service answer `/health`, and a suite pointed at the wrong container
 * on a shared staging network would otherwise pass this on someone else's
 * health handler.
 */
export const healthSchema = z
	.object({
		status: z.literal("ok"),
		service: z.literal("knowledge-sovereign"),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// Events (handler/rpc_infra.go, driver/sovereign_db/read_events.go)
// ---------------------------------------------------------------------------

/** `AppendKnowledgeEventResponse` for an event that was actually written. */
export const appendEventSchema = z.object({ eventSeq: positiveInt64Schema }).passthrough();

export const knowledgeEventSchema = z
	.object({
		eventId: uuidSchema,
		eventSeq: positiveInt64Schema,
		occurredAt: timestampSchema,
		tenantId: uuidSchema,
		// Omitted for tenant-wide system events, which the driver's
		// `(user_id = $3 OR user_id IS NULL)` predicate deliberately admits.
		userId: uuidSchema.optional(),
		actorType: z.string().optional(),
		actorId: z.string().optional(),
		eventType: z.string().min(1),
		aggregateType: z.string().optional(),
		aggregateId: z.string().optional(),
		correlationId: uuidSchema.optional(),
		causationId: uuidSchema.optional(),
		dedupeKey: z.string().optional(),
		// proto `bytes payload` — base64 in proto3-JSON, absent when empty.
		payload: z.string().optional(),
	})
	.passthrough();

export const listEventsSchema = z
	.object({ events: z.array(knowledgeEventSchema).optional() })
	.passthrough();

export const latestEventSeqSchema = z.object({ eventSeq: int64Schema }).passthrough();

// ---------------------------------------------------------------------------
// Projection infrastructure (handler/rpc_infra.go)
// ---------------------------------------------------------------------------

export const projectionVersionSchema = z
	.object({
		version: z.number().int().positive(),
		description: z.string().min(1),
		status: z.string().min(1),
		createdAt: timestampSchema,
		activatedAt: timestampSchema.optional(),
	})
	.passthrough();

export const activeProjectionVersionSchema = z
	.object({ version: projectionVersionSchema })
	.passthrough();

export const listProjectionVersionsSchema = z
	.object({ versions: z.array(projectionVersionSchema).min(1) })
	.passthrough();

/** `GetProjectionCheckpointResponse`; `lastEventSeq` is omitted while 0. */
export const projectionCheckpointSchema = z
	.object({ lastEventSeq: int64Schema.optional() })
	.passthrough();

/**
 * `GetProjectionLagResponse`.
 *
 * Both fields are proto `double`, so protojson emits them as numbers and
 * omits them when exactly 0 — which is the normal answer on an idle stack.
 */
export const projectionLagSchema = z
	.object({
		lagSeconds: z.number().optional(),
		ageSeconds: z.number().optional(),
	})
	.passthrough();

export const projectionFreshnessSchema = z
	.object({
		found: z.boolean().optional(),
		updatedAt: timestampSchema.optional(),
	})
	.passthrough();

export const listDistinctUserIdsSchema = z
	.object({ userIds: z.array(uuidSchema).optional() })
	.passthrough();

export const countNeedToKnowSchema = z
	.object({ count: z.number().int().nonnegative().optional() })
	.passthrough();

export const backfillJobSchema = z
	.object({
		jobId: uuidSchema,
		status: z.string().optional(),
		/**
		 * Never omitted, because `CreateBackfillJob` substitutes "articles" for
		 * an empty kind (ADR-000846's additive discriminator). A job that came
		 * back with no `kind` would mean that default was lost and every legacy
		 * producer's job had become unclassifiable.
		 */
		kind: z.string().min(1),
		projectionVersion: z.number().int().optional(),
		totalEvents: z.number().int().optional(),
		processedEvents: z.number().int().optional(),
		createdAt: timestampSchema,
		updatedAt: timestampSchema,
	})
	.passthrough();

export const getBackfillJobSchema = z.object({ job: backfillJobSchema }).passthrough();

export const listBackfillJobsSchema = z
	.object({ jobs: z.array(backfillJobSchema).optional() })
	.passthrough();

export const reprojectRunSchema = z
	.object({
		reprojectRunId: uuidSchema,
		projectionName: z.string().min(1),
		fromVersion: z.string().optional(),
		toVersion: z.string().optional(),
		mode: z.string().optional(),
		status: z.string().optional(),
		createdAt: timestampSchema,
	})
	.passthrough();

export const getReprojectRunSchema = z.object({ run: reprojectRunSchema }).passthrough();

export const listReprojectRunsSchema = z
	.object({ runs: z.array(reprojectRunSchema).optional() })
	.passthrough();

export const projectionAuditSchema = z
	.object({
		auditId: uuidSchema,
		projectionName: z.string().min(1),
		// proto declares this `string`, not int32 — a genuine asymmetry with
		// ProjectionVersion.version, and one a client will get wrong exactly
		// once.
		projectionVersion: z.string().optional(),
		checkedAt: timestampSchema,
		sampleSize: z.number().int().optional(),
		mismatchCount: z.number().int().optional(),
	})
	.passthrough();

export const listProjectionAuditsSchema = z
	.object({ audits: z.array(projectionAuditSchema).min(1) })
	.passthrough();

export const compareProjectionsSchema = z
	.object({
		summary: z
			.object({
				fromCount: z.number().int().nonnegative().optional(),
				toCount: z.number().int().nonnegative().optional(),
				fromAvgScore: z.number().optional(),
				toAvgScore: z.number().optional(),
				fromEmptySummary: z.number().int().nonnegative().optional(),
				toEmptySummary: z.number().int().nonnegative().optional(),
			})
			.passthrough(),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// Knowledge Home projections (handler/rpc_projections.go)
// ---------------------------------------------------------------------------

export const whyReasonSchema = z.object({ code: z.string().min(1) }).passthrough();

export const knowledgeHomeItemSchema = z
	.object({
		userId: uuidSchema,
		tenantId: uuidSchema,
		itemKey: z.string().min(1),
		itemType: z.string().min(1),
		primaryRefId: uuidSchema.optional(),
		title: z.string().optional(),
		summaryExcerpt: z.string().optional(),
		tags: z.array(z.string()).optional(),
		whyReasons: z.array(whyReasonSchema).optional(),
		score: z.number().optional(),
		url: z.string().optional(),
		generatedAt: timestampSchema,
		updatedAt: timestampSchema,
		/**
		 * Stamped by the projector from the row it resolved as `active`, and
		 * the read path filters on the same subquery
		 * (`driver/sovereign_db/sql_fragments.go`). A row that came back with
		 * a *different* version than the read filter asked for would mean the
		 * two disagree — hence asserting it is present and positive rather
		 * than ignoring it.
		 */
		projectionVersion: z.number().int().positive(),
		summaryState: z.string().optional(),
		supersedeState: z.string().optional(),
	})
	.passthrough();

export const homeItemsSchema = z
	.object({
		items: z.array(knowledgeHomeItemSchema).optional(),
		nextCursor: z.string().optional(),
		hasMore: z.boolean().optional(),
	})
	.passthrough();

export const homeItemsPopulatedSchema = z
	.object({
		items: z.array(knowledgeHomeItemSchema).min(1),
		nextCursor: z.string().optional(),
		hasMore: z.boolean().optional(),
	})
	.passthrough();

export const todayDigestSchema = z
	.object({
		userId: uuidSchema,
		digestDate: dateSchema,
		newArticles: z.number().int().nonnegative().optional(),
		summarizedArticles: z.number().int().nonnegative().optional(),
		unsummarizedArticles: z.number().int().nonnegative().optional(),
		topTags: z.array(z.string()).optional(),
		updatedAt: timestampSchema,
		weeklyRecapAvailable: z.boolean().optional(),
		eveningPulseAvailable: z.boolean().optional(),
	})
	.passthrough();

export const todayDigestResponseSchema = z
	.object({ digest: todayDigestSchema.optional() })
	.passthrough();

export const todayDigestFoundSchema = z.object({ digest: todayDigestSchema }).passthrough();

/**
 * The `item` a recall candidate carries is **not** a full `KnowledgeHomeItem`,
 * and deliberately asserting less here is the honest thing to do rather than a
 * shortcut.
 *
 * `GetRecallCandidates` builds it from the columns its LEFT JOIN selected and
 * leaves the rest at their zero values (read_projections.go: the literal only
 * sets UserID, ItemKey, Title and PrimaryRefID, then fills five more
 * conditionally). So `tenantId` arrives as the all-zero UUID,
 * `projectionVersion` as 0 — omitted by protojson — and the two timestamps as
 * `0001-01-01T00:00:00Z`.
 *
 * Reusing `knowledgeHomeItemSchema` here would fail on a wire shape that is
 * current, intended behaviour; using this narrower schema records that the
 * embedded item is a display preview, not the projection row, and that a
 * consumer must not read `projectionVersion` or `tenantId` off it.
 */
export const embeddedRecallItemSchema = z
	.object({
		userId: uuidSchema,
		itemKey: z.string().min(1),
		itemType: z.string().optional(),
		title: z.string().optional(),
		summaryExcerpt: z.string().optional(),
		url: z.string().optional(),
		primaryRefId: uuidSchema.optional(),
		tags: z.array(z.string()).optional(),
		whyReasons: z.array(whyReasonSchema).optional(),
		score: z.number().optional(),
	})
	.passthrough();

export const recallCandidateSchema = z
	.object({
		userId: uuidSchema,
		itemKey: z.string().min(1),
		recallScore: z.number().optional(),
		reasons: z
			.array(z.object({ type: z.string().min(1) }).passthrough())
			.optional(),
		nextSuggestAt: timestampSchema.optional(),
		firstEligibleAt: timestampSchema.optional(),
		updatedAt: timestampSchema,
		projectionVersion: z.number().int().positive(),
		item: embeddedRecallItemSchema.optional(),
	})
	.passthrough();

export const recallCandidatesSchema = z
	.object({ candidates: z.array(recallCandidateSchema).optional() })
	.passthrough();

export const recallCandidatesPopulatedSchema = z
	.object({ candidates: z.array(recallCandidateSchema).min(1) })
	.passthrough();

// ---------------------------------------------------------------------------
// Knowledge Trail spine (handler/rpc_trail.go)
// ---------------------------------------------------------------------------

export const trailFootprintSchema = z
	.object({
		userId: uuidSchema,
		tenantId: uuidSchema,
		footprintKey: z.string().min(1),
		verb: z.string().min(1),
		itemKey: z.string().min(1),
		title: z.string().optional(),
		excerpt: z.string().optional(),
		tags: z.array(z.string()).optional(),
		note: z.string().optional(),
		sourceEventType: z.string().min(1),
		occurredAt: timestampSchema,
		wear: z.string().optional(),
		// `max(fp.ContactCount, 1)` in mapTrailFootprints, so never 0 and
		// therefore never omitted by protojson.
		contactCount: z.number().int().positive(),
		firstOccurredAt: timestampSchema,
	})
	.passthrough();

export const trailEpisodeSchema = z
	.object({
		episodeKey: z.string().min(1),
		wear: z.string().optional(),
		footprints: z.array(trailFootprintSchema).min(1),
	})
	.passthrough();

export const trailBranchSchema = z
	.object({
		branchKey: z.string().min(1),
		anchorItemKey: z.string().min(1),
		relationKind: z.string().optional(),
		why: z.string().optional(),
		confidence: z.string().optional(),
	})
	.passthrough();

export const trailFootprintsSchema = z
	.object({
		/**
		 * Wave 8 superseded the flat `footprints` list with `episodes` and the
		 * handler now always leaves it empty — so protojson always omits it.
		 * Asserting it stays absent is the regression fence against a revert
		 * that would double-render every act in the SPA.
		 */
		footprints: z.undefined(),
		episodes: z.array(trailEpisodeSchema).optional(),
		branches: z.array(trailBranchSchema).optional(),
		nextCursor: z.string().optional(),
		hasMore: z.boolean().optional(),
	})
	.passthrough();

export const trailBranchesForAnchorSchema = z
	.object({ branches: z.array(trailBranchSchema).optional() })
	.passthrough();

// ---------------------------------------------------------------------------
// Lenses (handler/rpc_lens.go)
// ---------------------------------------------------------------------------

export const lensVersionSchema = z
	.object({
		lensVersionId: uuidSchema,
		lensId: uuidSchema,
		createdAt: timestampSchema,
		queryText: z.string().optional(),
		tagIds: z.array(z.string()).optional(),
		sourceIds: z.array(z.string()).optional(),
		timeWindow: z.string().optional(),
		includeRecap: z.boolean().optional(),
		includePulse: z.boolean().optional(),
		sortMode: z.string().optional(),
		supersededBy: uuidSchema.optional(),
	})
	.passthrough();

export const lensSchema = z
	.object({
		lensId: uuidSchema,
		userId: uuidSchema,
		tenantId: uuidSchema,
		name: z.string().min(1),
		description: z.string().optional(),
		createdAt: timestampSchema,
		updatedAt: timestampSchema,
		archivedAt: timestampSchema.optional(),
		currentVersion: lensVersionSchema.optional(),
	})
	.passthrough();

export const listLensesSchema = z.object({ lenses: z.array(lensSchema).optional() }).passthrough();

export const getLensSchema = z.object({ lens: lensSchema }).passthrough();

export const currentLensSelectionSchema = z
	.object({
		found: z.literal(true),
		selection: z
			.object({
				userId: uuidSchema,
				lensId: uuidSchema,
				lensVersionId: uuidSchema,
				selectedAt: timestampSchema,
			})
			.passthrough(),
	})
	.passthrough();

export const resolvedLensFilterSchema = z
	.object({
		found: z.literal(true),
		filter: z
			.object({
				queryText: z.string().optional(),
				tagIds: z.array(z.string()).optional(),
				sourceIds: z.array(z.string()).optional(),
				timeWindow: z.string().optional(),
				includeRecap: z.boolean().optional(),
				includePulse: z.boolean().optional(),
				sortMode: z.string().optional(),
			})
			.passthrough(),
	})
	.passthrough();

// ---------------------------------------------------------------------------
// Recall signals (handler/rpc_infra.go)
// ---------------------------------------------------------------------------

export const recallSignalSchema = z
	.object({
		signalId: uuidSchema,
		userId: uuidSchema,
		itemKey: z.string().min(1),
		signalType: z.string().min(1),
		signalStrength: z.number().optional(),
		occurredAt: timestampSchema,
		payload: z.string().optional(),
	})
	.passthrough();

export const listRecallSignalsSchema = z
	.object({ signals: z.array(recallSignalSchema).min(1) })
	.passthrough();

// ---------------------------------------------------------------------------
// /admin/* on :9501 — plain encoding/json, snake_case (ADR-000942)
// ---------------------------------------------------------------------------

/**
 * `sovereign_db.SnapshotMetadata`.
 *
 * Every field carries an explicit json tag and none has `omitempty`, so the
 * whole struct is on the wire every time — which is why nothing here is
 * `.optional()`. This is the shape `altctl home snapshot {list,latest}`
 * unmarshals into; before ADR-000942 the handler emitted PascalCase keys and
 * altctl could never decode it.
 */
export const snapshotMetadataSchema = z
	.object({
		snapshot_id: uuidSchema,
		snapshot_type: z.string().min(1),
		projection_version: z.number().int().positive(),
		projector_build_ref: z.string().min(1),
		schema_version: z.string().min(1),
		snapshot_at: timestampSchema,
		event_seq_boundary: z.number().int().positive(),
		snapshot_data_path: z.string().min(1),
		items_row_count: z.number().int().nonnegative(),
		items_checksum: sha256Schema,
		digest_row_count: z.number().int().nonnegative(),
		digest_checksum: sha256Schema,
		recall_row_count: z.number().int().nonnegative(),
		recall_checksum: sha256Schema,
		created_at: timestampSchema,
		status: z.string().min(1),
	})
	.passthrough();

export const snapshotListSchema = z
	.object({ snapshots: z.array(snapshotMetadataSchema).min(1) })
	.passthrough();

export const retentionActionSchema = z
	.object({
		action: z.string().min(1),
		table: z.string().min(1),
		partition: z.string().min(1),
		rows: z.number().int().nonnegative(),
		path: z.string().optional(),
		checksum: z.string().optional(),
		status: z.string().min(1),
	})
	.passthrough();

/**
 * `retentionRunResponse`.
 *
 * `Actions []retentionAction` has no `omitempty`, so a run that planned
 * nothing marshals it as JSON **null**, not `[]` — `.nullable()` rather than
 * `.optional()`. The Hurl suite asserted only `dry_run == true` and the
 * absence of `error`, so this null/array split was never pinned anywhere.
 */
export const retentionRunSchema = z
	.object({
		dry_run: z.boolean(),
		actions: z.array(retentionActionSchema).nullable(),
		error: z.string().optional(),
	})
	.passthrough();

export const retentionLogEntrySchema = z
	.object({
		log_id: uuidSchema,
		run_at: timestampSchema,
		action: z.string().min(1),
		target_table: z.string().min(1),
		target_partition: z.string(),
		rows_affected: z.number().int(),
		dry_run: z.boolean(),
		status: z.string().min(1),
	})
	.passthrough();

export const retentionStatusSchema = z
	.object({ logs: z.array(retentionLogEntrySchema) })
	.passthrough();

export const eligiblePartitionSchema = z
	.object({
		table_name: z.string().min(1),
		partition_name: z.string().min(1),
		range_start: timestampSchema,
		range_end: timestampSchema,
		row_count: z.number().int().nonnegative(),
		size_bytes: z.number().int().nonnegative(),
	})
	.passthrough();

export const eligiblePartitionsSchema = z
	.object({ partitions: z.array(eligiblePartitionSchema) })
	.passthrough();

/**
 * `sovereign_db.TableStorageInfo` — note the Go field is `TableName` but the
 * json tag is `name`, which is the sort of drift a schema catches and a
 * `jsonpath "$.tables[0].name" exists` spot check does not.
 */
export const tableStorageInfoSchema = z
	.object({
		name: z.string().min(1),
		row_count: z.number().int().nonnegative(),
		total_size: z.string().min(1),
		table_size: z.string().min(1),
		index_size: z.string().min(1),
		total_bytes: z.number().int().nonnegative(),
		is_partitioned: z.boolean(),
	})
	.passthrough();

export const storageStatsSchema = z
	.object({ tables: z.array(tableStorageInfoSchema).min(1) })
	.passthrough();

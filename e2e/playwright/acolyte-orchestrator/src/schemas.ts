import { z } from "zod";
import { timestampSchema, uuidSchema } from "../../_shared/schemas.js";

/**
 * The shapes alt.acolyte.v1.AcolyteService promises, as zod schemas.
 *
 * The Hurl suite these replace could only assert one JSONPath at a time, so
 * "the response is a report" was spelled as four independent `jsonpath`
 * comparisons and everything they did not mention was unchecked. A schema
 * asserts the *whole* envelope in one step: a handler that starts emitting
 * `created_at` in snake_case, or drops `sections` entirely, fails here instead
 * of quietly satisfying the fields the old file happened to name.
 *
 * # Why nearly every field is `.optional()`
 *
 * connect-python marshals with protobuf's canonical proto3 JSON, which **omits
 * fields at their zero value**. A brand-new report really does come back as
 * `{"report":{"reportId":…,"title":…,"reportType":…,"createdAt":…}}` with no
 * `currentVersion` key at all, because it is 0. Marking those `.optional()` is
 * therefore not laxity — it is the wire format. Where a value cannot be zero in
 * a valid response (`versionNo` starts at 1; `citationsJson` is at minimum the
 * two-character string `"[]"`, connect_service.py:102) the schema requires it,
 * which is what keeps "optional" from spreading into "unchecked".
 *
 * Every object is `.passthrough()`: these are contracts on the fields the
 * service promises, not a freeze on the ones the proto may add.
 */

export { uuidSchema, timestampSchema } from "../../_shared/schemas.js";
export { connectErrorSchema } from "../../_shared/connect.js";

/** `GET /health` — main.py:123-125 returns exactly these two keys. */
export const restHealthSchema = z
	.object({
		status: z.literal("ok"),
		service: z.literal("acolyte-orchestrator"),
	})
	.passthrough();

/** `HealthCheck` RPC — connect_service.py:439-442. */
export const healthCheckResponseSchema = z.object({ status: z.literal("ok") }).passthrough();

/**
 * The run_status domain, pinned to the CHECK constraint on `report_runs`
 * (acolyte-migration-atlas/migrations/20260409000000_create_acolyte_tables.sql:68-69).
 *
 * Worth spelling out as an enum rather than `z.string()`: the terminal value is
 * `succeeded`, and the orchestrator's *log messages* say "Pipeline completed"
 * (connect_service.py:345). A test that accepted any string would not notice a
 * handler that started reporting the log's word instead of the column's.
 */
export const runStatusSchema = z.enum([
	"pending",
	"running",
	"succeeded",
	"failed",
	"cancelled",
]);

export const createReportResponseSchema = z.object({ reportId: uuidSchema }).passthrough();

export const startReportRunResponseSchema = z.object({ runId: uuidSchema }).passthrough();

/**
 * `RerunSection` is synchronous in the current handler and returns
 * `run_id=""` (connect_service.py:415), which proto3 JSON omits — so a correct
 * response is the empty object. `runId` stays declared-and-optional so that the
 * day the handler starts spawning a real run, the schema still parses and the
 * assertion in the spec is the thing that has to be updated.
 */
export const rerunSectionResponseSchema = z.object({ runId: uuidSchema.optional() }).passthrough();

/** `DeleteReportResponse` has no fields at all — proto3 JSON renders `{}`. */
export const deleteReportResponseSchema = z.object({}).passthrough();

export const reportSchema = z
	.object({
		reportId: uuidSchema,
		// Not `.min(1)`: the proto allows an empty title and CreateReport does
		// not reject one, so requiring it here would be asserting a validation
		// rule the service does not have.
		title: z.string(),
		reportType: z.string(),
		currentVersion: z.number().int().positive().optional(),
		latestSuccessfulRunId: z.string().optional(),
		createdAt: timestampSchema,
		/**
		 * `map<string,string> scope`, reconstructed from the report_briefs row
		 * by `ReportBrief.to_scope()` (domain/brief.py:71-82). Absent — not
		 * empty — when the report was created without a `topic`, because
		 * connect_service.py:79 only inserts a brief in that case.
		 */
		scope: z.record(z.string()).optional(),
	})
	.passthrough();

export const reportSectionSchema = z
	.object({
		sectionKey: z.string().min(1),
		currentVersion: z.number().int().nonnegative().optional(),
		displayOrder: z.number().int().nonnegative().optional(),
		body: z.string().optional(),
		// Always present: connect_service.py:102 falls back to the literal
		// "[]" rather than the empty string, so proto3 never omits it.
		citationsJson: z.string().min(1),
	})
	.passthrough();

export const reportRunSchema = z
	.object({
		runId: uuidSchema,
		reportId: uuidSchema,
		// target_version_no is current_version + 1, so it is never 0 on a real
		// run row (postgres_job_gw.create_run via start_run_uc.py:76).
		targetVersionNo: z.number().int().positive(),
		runStatus: runStatusSchema,
		plannerModel: z.string().optional(),
		writerModel: z.string().optional(),
		criticModel: z.string().optional(),
		startedAt: z.string().optional(),
		finishedAt: z.string().optional(),
		failureCode: z.string().optional(),
		failureMessage: z.string().optional(),
	})
	.passthrough();

export const getReportResponseSchema = z
	.object({
		report: reportSchema,
		sections: z.array(reportSectionSchema).optional(),
		/**
		 * Populated only while a pending/running run exists for this report
		 * (connect_service.py:106-124, backed by
		 * `get_active_run_for_report`, whose SQL filters
		 * `run_status IN ('pending','running')`). Its *absence* after a
		 * terminal run is a contract in its own right — the SPA polls on it.
		 */
		activeRun: reportRunSchema.optional(),
	})
	.passthrough();

/**
 * `GetRunStatus` fills only four fields of ReportRun
 * (connect_service.py:370-377) — no models, no timestamps. The full-fat
 * ReportRun only ever appears as `GetReportResponse.activeRun`.
 */
export const getRunStatusResponseSchema = z
	.object({
		run: reportRunSchema,
		jobs: z.array(z.record(z.unknown())).optional(),
	})
	.passthrough();

export const reportSummarySchema = z
	.object({
		reportId: uuidSchema,
		title: z.string(),
		reportType: z.string(),
		currentVersion: z.number().int().positive().optional(),
		/** "" (omitted) until a run row exists — connect_service.py:150-154. */
		latestRunStatus: runStatusSchema.optional(),
		createdAt: timestampSchema,
	})
	.passthrough();

export const listReportsResponseSchema = z
	.object({
		reports: z.array(reportSummarySchema).optional(),
		nextCursor: z.string().optional(),
		hasMore: z.literal(true).optional(),
	})
	.passthrough();

export const changeItemSchema = z
	.object({
		fieldName: z.string().optional(),
		changeKind: z.enum(["added", "updated", "removed", "regenerated"]).optional(),
		oldFingerprint: z.string().optional(),
		newFingerprint: z.string().optional(),
	})
	.passthrough();

export const reportVersionSummarySchema = z
	.object({
		// Version numbers start at 1 (bump_version writes current_version + 1),
		// so a 0 here would mean the projection lost the value, not that proto3
		// omitted a legitimate zero.
		versionNo: z.number().int().positive(),
		changeReason: z.string().optional(),
		createdAt: timestampSchema.optional(),
		changeItems: z.array(changeItemSchema).optional(),
	})
	.passthrough();

export const listReportVersionsResponseSchema = z
	.object({
		versions: z.array(reportVersionSummarySchema).optional(),
		nextCursor: z.string().optional(),
		hasMore: z.literal(true).optional(),
	})
	.passthrough();

/**
 * `hasMore` is declared as `z.literal(true).optional()` above rather than
 * `z.boolean().optional()` on purpose: proto3 JSON omits `false`, so a literal
 * `false` on the wire would mean the server switched to
 * `including_default_value_fields`, which changes every "absent means zero"
 * assumption in this file at once. Better to hear about that here.
 */
export type ListReportsResponse = z.infer<typeof listReportsResponseSchema>;
export type GetReportResponse = z.infer<typeof getReportResponseSchema>;
export type GetRunStatusResponse = z.infer<typeof getRunStatusResponseSchema>;

import type { Page } from "@playwright/test";
import { daysAgo, fulfillJson, hoursAgo } from "../helpers";

export const ACOLYTE_REPORT_ID = "019a9f00-6000-7000-8000-000000000001";

export const ACOLYTE_RPCS = {
	getReport: "**/api/v2/alt.acolyte.v1.AcolyteService/GetReport",
	listReportVersions:
		"**/api/v2/alt.acolyte.v1.AcolyteService/ListReportVersions",
	listReports: "**/api/v2/alt.acolyte.v1.AcolyteService/ListReports",
} as const;

export const ACOLYTE_REPORT_TITLE =
	"How append-only designs change database maintenance";

export const ACOLYTE_EXPECTED_TEXTS = [
	ACOLYTE_REPORT_TITLE,
	"Edition 2",
	"executive summary",
];

export const ACOLYTE_REPORT_RESPONSE = {
	report: {
		reportId: ACOLYTE_REPORT_ID,
		title: ACOLYTE_REPORT_TITLE,
		reportType: "research_brief",
		currentVersion: 2,
		latestSuccessfulRunId: "019a9f00-6100-7000-8000-000000000001",
		createdAt: hoursAgo(3),
		scope: {
			topic: "Append-only event logs and storage maintenance",
			time_range: "Last 7 days",
			entities: "PostgreSQL, Rust, Go",
		},
	},
	sections: [
		{
			sectionKey: "executive_summary",
			currentVersion: 2,
			displayOrder: 0,
			body: [
				"Append-only architectures shift database maintenance from mutating in-place rows to managing immutable event partitions and deterministic read projections [1]. When every projection is derived from an ordered log, schema changes become offline or shadow-table replays rather than risky live migrations.",
				"Across recent engineering reports, teams pair bounded autovacuum workers with incremental materialized views so foreground read latency stays flat during heavy write bursts [2].",
			].join("\n\n"),
			citationsJson: JSON.stringify([
				{
					claim_id: "1",
					source_id:
						"Example Dev Weekly: Incremental materialized views over append-only event logs",
					source_type: "article",
					quote:
						"Shadow projections let teams replay immutable streams from offset zero without taking live readers offline.",
					offset_start: 0,
					offset_end: 103,
				},
				{
					claim_id: "2",
					source_id:
						"Example Dev Weekly: Tuning PostgreSQL vacuum workers for heavy writes",
					source_type: "article",
					quote:
						"Splitting hot append tables into shorter partitions keeps dead tuple bloat bounded under sustained writes.",
					offset_start: 0,
					offset_end: 105,
				},
				{
					claim_id: "3",
					source_id:
						"Example Systems Blog: Structured concurrency patterns for Go worker pipelines",
					source_type: "article",
					quote:
						"Checkpointed worker pools isolate replay backpressure from foreground API handlers.",
					offset_start: 0,
					offset_end: 84,
				},
			]),
		},
		{
			sectionKey: "projection_rebuilds",
			currentVersion: 2,
			displayOrder: 1,
			body: [
				"Rebuilding a disposable read model starts by allocating a shadow table and scanning historical log segments in bounded batches [1].",
				"Once the replay cursor reaches the live tail, dual-write verification checks row counts and hashes before swapping reader pointers [3].",
			].join("\n\n"),
			citationsJson: JSON.stringify([
				{
					claim_id: "1",
					source_id:
						"Example Dev Weekly: Incremental materialized views over append-only event logs",
					source_type: "article",
					quote:
						"Shadow projections let teams replay immutable streams from offset zero without taking live readers offline.",
					offset_start: 0,
					offset_end: 103,
				},
			]),
		},
		{
			sectionKey: "vacuum_and_write_paths",
			currentVersion: 2,
			displayOrder: 2,
			body: "Partitioning append tables by time window allows whole segments to be archived or dropped without scanning live indexes [2]. Tuning autovacuum cost limits prevents transaction ID wraparound while preserving predictable p99 query latency.",
			citationsJson: JSON.stringify([
				{
					claim_id: "2",
					source_id:
						"Example Dev Weekly: Tuning PostgreSQL vacuum workers for heavy writes",
					source_type: "article",
					quote:
						"Splitting hot append tables into shorter partitions keeps dead tuple bloat bounded under sustained writes.",
					offset_start: 0,
					offset_end: 105,
				},
			]),
		},
		{
			sectionKey: "operational_tradeoffs",
			currentVersion: 2,
			displayOrder: 3,
			body: [
				"Keeping both the raw event stream and materialized projections increases disk footprint and requires strict idempotency keys on consumers [3].",
				"Teams mitigate storage growth by compacting closed partitions after downstream projectors confirm durable checkpoints [4].",
			].join("\n\n"),
			citationsJson: JSON.stringify([
				{
					claim_id: "4",
					source_id:
						"Example Systems Blog: Zero-copy ring buffers for faster Rust services",
					source_type: "article",
					quote:
						"Batch thresholds aligned with cache-line boundaries keep high-volume log ingestion overhead low.",
					offset_start: 0,
					offset_end: 95,
				},
			]),
		},
	],
};

export const ACOLYTE_VERSIONS_RESPONSE = {
	versions: [
		{
			versionNo: 2,
			changeReason:
				"Expanded vacuum worker bounds and added shadow-table replay evidence.",
			createdAt: hoursAgo(3),
			changeItems: [
				{
					fieldName: "executive_summary",
					changeKind: "updated",
					oldFingerprint: "v1-exec",
					newFingerprint: "v2-exec",
				},
				{
					fieldName: "operational_tradeoffs",
					changeKind: "added",
					oldFingerprint: "",
					newFingerprint: "v2-ops",
				},
			],
		},
		{
			versionNo: 1,
			changeReason: "Initial brief compiled from recent storage articles.",
			createdAt: daysAgo(1),
			changeItems: [],
		},
	],
	nextCursor: "",
	hasMore: false,
};

export async function mockAcolyteRoutes(page: Page): Promise<void> {
	await Promise.all([
		page.route(ACOLYTE_RPCS.getReport, (route) =>
			fulfillJson(route, ACOLYTE_REPORT_RESPONSE),
		),
		page.route(ACOLYTE_RPCS.listReportVersions, (route) =>
			fulfillJson(route, ACOLYTE_VERSIONS_RESPONSE),
		),
		page.route(ACOLYTE_RPCS.listReports, (route) =>
			fulfillJson(route, { reports: [], nextCursor: "", hasMore: false }),
		),
	]);
}

/**
 * Sovereign Admin REST API Contract Tests
 *
 * Pins the wire shape of knowledge-sovereign's /admin/* endpoints (metrics
 * port :9501) against the real translation in $lib/server/sovereign-admin-wire.
 *
 * Fixtures are transcribed from the Go source, handler -> response struct ->
 * json tags:
 *   handler/storage_handler.go    storageStatsResponse{tables}
 *   handler/snapshot_handler.go   snapshotListResponse{snapshots}, bare object
 *   handler/retention_handler.go  retentionStatusResponse{logs},
 *                                 eligiblePartitionsResponse{partitions},
 *                                 retentionRunResponse{dry_run,actions,error}
 *   driver/sovereign_db/snapshot.go   SnapshotMetadata, TableStorageInfo
 *   driver/sovereign_db/retention.go  RetentionLogEntry, PartitionInfo
 *
 * Every row is snake_case and every list endpoint wraps its rows in a named
 * envelope. Nothing here may re-implement a normalizer: a contract test that
 * declares its own copy of the code under test is green by construction.
 *
 * The request side (bearer token, dry_run body, error statuses) is pinned in
 * src/lib/server/sovereign-admin.test.ts, which vitest runs: that module reads
 * $env/dynamic/private, which the bun runner used for this directory cannot
 * resolve.
 */
import { describe, expect, it } from "vitest";
import {
	normalizeRetentionRun,
	normalizeSnapshotMetadata,
	normalizeSovereignAdminSnapshot,
	type SovereignAdminWire,
} from "$lib/server/sovereign-admin-wire";

// driver/sovereign_db/snapshot.go SnapshotMetadata — all 16 fields, no omitempty.
const WIRE_SNAPSHOT = {
	snapshot_id: "7c9e6679-7425-40de-944b-e07fc1f90ae7",
	snapshot_type: "full",
	projection_version: 4,
	projector_build_ref: "sha-3d51e2c",
	schema_version: "00051",
	snapshot_at: "2026-08-12T03:30:00Z",
	event_seq_boundary: 1204873,
	snapshot_data_path:
		"/var/lib/knowledge-sovereign/snapshots/snapshot_20260812_033000",
	items_row_count: 18402,
	items_checksum: "sha256:0b1c2d3e4f50617283940a1b2c3d4e5f",
	digest_row_count: 30,
	digest_checksum: "sha256:5f4e3d2c1b0a90817263544f3e2d1c0b",
	recall_row_count: 640,
	recall_checksum: "sha256:aabbccddeeff00112233445566778899",
	created_at: "2026-08-12T03:30:11Z",
	status: "valid",
};

const VIEW_SNAPSHOT = {
	snapshotId: "7c9e6679-7425-40de-944b-e07fc1f90ae7",
	snapshotType: "full",
	projectionVersion: 4,
	projectorBuildRef: "sha-3d51e2c",
	schemaVersion: "00051",
	snapshotAt: "2026-08-12T03:30:00Z",
	eventSeqBoundary: 1204873,
	snapshotDataPath:
		"/var/lib/knowledge-sovereign/snapshots/snapshot_20260812_033000",
	itemsRowCount: 18402,
	itemsChecksum: "sha256:0b1c2d3e4f50617283940a1b2c3d4e5f",
	digestRowCount: 30,
	digestChecksum: "sha256:5f4e3d2c1b0a90817263544f3e2d1c0b",
	recallRowCount: 640,
	recallChecksum: "sha256:aabbccddeeff00112233445566778899",
	createdAt: "2026-08-12T03:30:11Z",
	status: "valid",
};

// driver/sovereign_db/snapshot.go TableStorageInfo — the table name ships as
// `name`, and table_size / index_size have no counterpart in the view type.
const WIRE_STORAGE_STATS = {
	tables: [
		{
			name: "knowledge_events",
			row_count: 1204873,
			total_size: "982 MB",
			table_size: "801 MB",
			index_size: "181 MB",
			total_bytes: 1029701632,
			is_partitioned: true,
		},
		{
			name: "today_digest_view",
			row_count: 30,
			total_size: "128 kB",
			table_size: "96 kB",
			index_size: "32 kB",
			total_bytes: 131072,
			is_partitioned: false,
		},
	],
};

// driver/sovereign_db/retention.go RetentionLogEntry — archive_path, checksum
// and error_message carry omitempty, so a dry-run row omits the keys entirely.
const WIRE_RETENTION_STATUS = {
	logs: [
		{
			log_id: "1f0a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8",
			run_at: "2026-08-11T04:00:00Z",
			action: "export",
			target_table: "knowledge_events",
			target_partition: "knowledge_events_y2025m12",
			rows_affected: 51120,
			archive_path:
				"/var/lib/knowledge-sovereign/archives/knowledge_events_y2025m12_20260811.jsonl.gz",
			checksum: "sha256:112233445566778899aabbccddeeff00",
			dry_run: false,
			status: "exported",
		},
		{
			log_id: "2e1b3c4d-5e6f-7081-92a3-b4c5d6e7f809",
			run_at: "2026-08-10T04:00:00Z",
			action: "export",
			target_table: "knowledge_user_events",
			target_partition: "knowledge_user_events_y2025m12",
			rows_affected: 0,
			dry_run: true,
			status: "dry_run",
		},
	],
};

// handler/retention_handler.go eligiblePartitionRow — one flat, table-tagged
// array, not a per-table grouping.
const WIRE_ELIGIBLE = {
	partitions: [
		{
			table_name: "knowledge_events",
			partition_name: "knowledge_events_y2025m12",
			range_start: "2025-12-01T00:00:00Z",
			range_end: "2026-01-01T00:00:00Z",
			row_count: 51120,
			size_bytes: 63963136,
		},
		{
			table_name: "knowledge_user_events",
			partition_name: "knowledge_user_events_y2025m12",
			range_start: "2025-12-01T00:00:00Z",
			range_end: "2026-01-01T00:00:00Z",
			row_count: 9340,
			size_bytes: 12058624,
		},
		{
			table_name: "knowledge_events",
			partition_name: "knowledge_events_y2026m01",
			range_start: "2026-01-01T00:00:00Z",
			range_end: "2026-02-01T00:00:00Z",
			row_count: 57841,
			size_bytes: 71303168,
		},
	],
};

function wireBodies(
	overrides: Partial<SovereignAdminWire> = {},
): SovereignAdminWire {
	return {
		storage: WIRE_STORAGE_STATS,
		snapshotList: { snapshots: [WIRE_SNAPSHOT] },
		latestSnapshot: WIRE_SNAPSHOT,
		retentionStatus: WIRE_RETENTION_STATUS,
		eligible: WIRE_ELIGIBLE,
		...overrides,
	};
}

describe("GET /admin/storage/stats", () => {
	it("unwraps the tables envelope and reads the table name from `name`", () => {
		const { storageStats } = normalizeSovereignAdminSnapshot(wireBodies());

		expect(storageStats).toEqual([
			{
				table_name: "knowledge_events",
				row_count: 1204873,
				total_size: "982 MB",
				total_bytes: 1029701632,
				is_partitioned: true,
			},
			{
				table_name: "today_digest_view",
				row_count: 30,
				total_size: "128 kB",
				total_bytes: 131072,
				is_partitioned: false,
			},
		]);
	});

	it("survives a table list the handler emitted as an empty array", () => {
		const { storageStats } = normalizeSovereignAdminSnapshot(
			wireBodies({ storage: { tables: [] } }),
		);

		expect(storageStats).toEqual([]);
	});
});

describe("GET /admin/snapshots/list and /admin/snapshots/latest", () => {
	it("maps every snake_case snapshot field to its camelCase view field", () => {
		const { snapshots, latestSnapshot } = normalizeSovereignAdminSnapshot(
			wireBodies(),
		);

		expect(snapshots).toHaveLength(1);
		expect(latestSnapshot).toEqual(VIEW_SNAPSHOT);
		expect(snapshots[0]).toEqual(latestSnapshot);
	});

	// handleGetLatestSnapshot encodes a nil *SnapshotMetadata, which is the JSON
	// literal null rather than an object or an envelope.
	it("reads a bare null from /snapshots/latest as no snapshot", () => {
		const { latestSnapshot } = normalizeSovereignAdminSnapshot(
			wireBodies({ latestSnapshot: null }),
		);

		expect(latestSnapshot).toBeNull();
	});
});

describe("POST /admin/snapshots/create", () => {
	it("reads the un-enveloped snapshot object the handler encodes", () => {
		expect(normalizeSnapshotMetadata(WIRE_SNAPSHOT)).toEqual(VIEW_SNAPSHOT);
	});
});

describe("GET /admin/retention/status", () => {
	it("unwraps the logs envelope and maps every field", () => {
		const { retentionLogs } = normalizeSovereignAdminSnapshot(wireBodies());

		expect(retentionLogs[0]).toEqual({
			logId: "1f0a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8",
			runAt: "2026-08-11T04:00:00Z",
			action: "export",
			targetTable: "knowledge_events",
			targetPartition: "knowledge_events_y2025m12",
			rowsAffected: 51120,
			archivePath:
				"/var/lib/knowledge-sovereign/archives/knowledge_events_y2025m12_20260811.jsonl.gz",
			checksum: "sha256:112233445566778899aabbccddeeff00",
			dryRun: false,
			status: "exported",
			errorMessage: "",
		});
	});

	it("fills the omitempty archive_path / checksum / error_message with empty strings", () => {
		const { retentionLogs } = normalizeSovereignAdminSnapshot(wireBodies());

		const dryRunLog = retentionLogs[1];
		expect(dryRunLog?.dryRun).toBe(true);
		expect(dryRunLog?.archivePath).toBe("");
		expect(dryRunLog?.checksum).toBe("");
		expect(dryRunLog?.errorMessage).toBe("");
	});
});

describe("GET /admin/retention/eligible", () => {
	it("regroups the flat table-tagged partition rows by table", () => {
		const { eligiblePartitions } = normalizeSovereignAdminSnapshot(
			wireBodies(),
		);

		expect(eligiblePartitions).toEqual([
			{
				table: "knowledge_events",
				eligible: [
					{
						name: "knowledge_events_y2025m12",
						rangeStart: "2025-12-01T00:00:00Z",
						rangeEnd: "2026-01-01T00:00:00Z",
						rowCount: 51120,
						sizeBytes: 63963136,
					},
					{
						name: "knowledge_events_y2026m01",
						rangeStart: "2026-01-01T00:00:00Z",
						rangeEnd: "2026-02-01T00:00:00Z",
						rowCount: 57841,
						sizeBytes: 71303168,
					},
				],
			},
			{
				table: "knowledge_user_events",
				eligible: [
					{
						name: "knowledge_user_events_y2025m12",
						rangeStart: "2025-12-01T00:00:00Z",
						rangeEnd: "2026-01-01T00:00:00Z",
						rowCount: 9340,
						sizeBytes: 12058624,
					},
				],
			},
		]);
	});

	it("returns no groups when the handler found nothing eligible", () => {
		const { eligiblePartitions } = normalizeSovereignAdminSnapshot(
			wireBodies({ eligible: { partitions: [] } }),
		);

		expect(eligiblePartitions).toEqual([]);
	});
});

describe("POST /admin/retention/run", () => {
	it("reads the snake_case action rows of a dry run", () => {
		const result = normalizeRetentionRun({
			dry_run: true,
			actions: [
				{
					action: "export",
					table: "knowledge_events",
					partition: "knowledge_events_y2025m12",
					rows: 0,
					status: "dry_run",
				},
			],
		});

		expect(result.dry_run).toBe(true);
		expect(result.actions).toEqual([
			{
				action: "export",
				table: "knowledge_events",
				partition: "knowledge_events_y2025m12",
				rows: 0,
				status: "dry_run",
			},
		]);
	});

	// retentionAction tags path and checksum omitempty, so only a live export
	// carries them.
	it("reads the omitempty path and checksum on a live export action", () => {
		const result = normalizeRetentionRun({
			dry_run: false,
			actions: [
				{
					action: "export",
					table: "knowledge_events",
					partition: "knowledge_events_y2025m12",
					rows: 51120,
					path: "/var/lib/knowledge-sovereign/archives/knowledge_events_y2025m12_20260812.jsonl.gz",
					checksum: "sha256:112233445566778899aabbccddeeff00",
					status: "exported",
				},
			],
		});

		expect(result.dry_run).toBe(false);
		expect(result.actions[0]?.rows).toBe(51120);
		expect(result.actions[0]?.path).toContain("y2025m12_20260812.jsonl.gz");
	});

	// The handler answers 200 with the error text in the body when RunRetention
	// fails its "no valid snapshot" precondition, and the response it built
	// before failing still carries the empty Actions slice it starts from.
	it("keeps the error text alongside the empty actions array", () => {
		const result = normalizeRetentionRun({
			dry_run: false,
			actions: [],
			error:
				"no valid snapshot found; create a snapshot before running retention",
		});

		expect(result.error).toBe(
			"no valid snapshot found; create a snapshot before running retention",
		);
		expect(result.actions).toEqual([]);
	});

	// RunRetention seeds Actions with an empty slice today, so null is not a
	// shape the handler emits; the panel still reads .length unconditionally, so
	// pin that the client stays total if that initializer is ever dropped.
	it("turns a null actions slice into an empty array", () => {
		const result = normalizeRetentionRun({ dry_run: true, actions: null });

		expect(result.actions).toEqual([]);
		expect(result.actions.length).toBe(0);
	});
});

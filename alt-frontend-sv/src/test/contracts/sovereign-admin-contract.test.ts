/**
 * Sovereign Admin REST API Contract Tests
 *
 * Pins the wire shape of knowledge-sovereign's /admin/* endpoints (metrics
 * port :9501) against the real client in $lib/server/sovereign-admin.
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
 */
import { afterAll, beforeEach, describe, expect, it, vi } from "vitest";

const { env } = vi.hoisted(() => ({
	env: {} as Record<string, string | undefined>,
}));
vi.mock("$env/dynamic/private", () => ({ env }));

const METRICS_URL = "http://knowledge-sovereign:9501";

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

const GET_BODIES: Record<string, unknown> = {
	"/admin/storage/stats": WIRE_STORAGE_STATS,
	"/admin/snapshots/list": { snapshots: [WIRE_SNAPSHOT] },
	"/admin/snapshots/latest": WIRE_SNAPSHOT,
	"/admin/retention/status": WIRE_RETENTION_STATUS,
	"/admin/retention/eligible": WIRE_ELIGIBLE,
};

function jsonResponse(body: unknown) {
	return { ok: true, status: 200, json: () => Promise.resolve(body) };
}

function stubGets(overrides: Record<string, unknown> = {}) {
	const bodies = { ...GET_BODIES, ...overrides };
	vi.stubGlobal(
		"fetch",
		vi.fn((url: string) => {
			const path = new URL(url).pathname;
			if (!(path in bodies)) {
				throw new Error(`unexpected fetch: ${url}`);
			}
			return Promise.resolve(jsonResponse(bodies[path]));
		}),
	);
}

async function importClient() {
	return import("$lib/server/sovereign-admin");
}

beforeEach(() => {
	vi.resetModules();
	vi.unstubAllGlobals();
	for (const key of Object.keys(env)) delete env[key];
	env.SOVEREIGN_METRICS_URL = METRICS_URL;
	env.SOVEREIGN_ADMIN_TOKEN = "contract-test-admin-token";
});

afterAll(() => {
	vi.unstubAllGlobals();
});

describe("GET /admin/storage/stats", () => {
	it("unwraps the tables envelope and reads the table name from `name`", async () => {
		stubGets();
		const { fetchSovereignAdminSnapshot } = await importClient();

		const { storageStats } = await fetchSovereignAdminSnapshot();

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

	it("survives a table list the handler emitted as an empty array", async () => {
		stubGets({ "/admin/storage/stats": { tables: [] } });
		const { fetchSovereignAdminSnapshot } = await importClient();

		const { storageStats } = await fetchSovereignAdminSnapshot();

		expect(storageStats).toEqual([]);
	});
});

describe("GET /admin/snapshots/list and /admin/snapshots/latest", () => {
	it("maps every snake_case snapshot field to its camelCase view field", async () => {
		stubGets();
		const { fetchSovereignAdminSnapshot } = await importClient();

		const { snapshots, latestSnapshot } = await fetchSovereignAdminSnapshot();

		expect(snapshots).toHaveLength(1);
		expect(latestSnapshot).toEqual({
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
		});
		expect(snapshots[0]).toEqual(latestSnapshot);
	});

	// handleGetLatestSnapshot encodes a nil *SnapshotMetadata, which is the JSON
	// literal null rather than an object or an envelope.
	it("reads a bare null from /snapshots/latest as no snapshot", async () => {
		stubGets({ "/admin/snapshots/latest": null });
		const { fetchSovereignAdminSnapshot } = await importClient();

		const { latestSnapshot } = await fetchSovereignAdminSnapshot();

		expect(latestSnapshot).toBeNull();
	});
});

describe("POST /admin/snapshots/create", () => {
	it("reads the un-enveloped snapshot object the handler encodes", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(jsonResponse(WIRE_SNAPSHOT)),
		);
		const { createSovereignSnapshot } = await importClient();

		const created = await createSovereignSnapshot();

		expect(created.snapshotId).toBe("7c9e6679-7425-40de-944b-e07fc1f90ae7");
		expect(created.eventSeqBoundary).toBe(1204873);
		expect(created.recallRowCount).toBe(640);
		expect(created.status).toBe("valid");
	});
});

describe("GET /admin/retention/status", () => {
	it("unwraps the logs envelope and maps every field", async () => {
		stubGets();
		const { fetchSovereignAdminSnapshot } = await importClient();

		const { retentionLogs } = await fetchSovereignAdminSnapshot();

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

	it("fills the omitempty archive_path / checksum / error_message with empty strings", async () => {
		stubGets();
		const { fetchSovereignAdminSnapshot } = await importClient();

		const { retentionLogs } = await fetchSovereignAdminSnapshot();

		const dryRunLog = retentionLogs[1];
		expect(dryRunLog?.dryRun).toBe(true);
		expect(dryRunLog?.archivePath).toBe("");
		expect(dryRunLog?.checksum).toBe("");
		expect(dryRunLog?.errorMessage).toBe("");
	});
});

describe("GET /admin/retention/eligible", () => {
	it("regroups the flat table-tagged partition rows by table", async () => {
		stubGets();
		const { fetchSovereignAdminSnapshot } = await importClient();

		const { eligiblePartitions } = await fetchSovereignAdminSnapshot();

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

	it("returns no groups when the handler found nothing eligible", async () => {
		stubGets({ "/admin/retention/eligible": { partitions: [] } });
		const { fetchSovereignAdminSnapshot } = await importClient();

		const { eligiblePartitions } = await fetchSovereignAdminSnapshot();

		expect(eligiblePartitions).toEqual([]);
	});
});

describe("POST /admin/retention/run", () => {
	it("sends dry_run and reads the snake_case action rows", async () => {
		const fetchMock = vi.fn().mockResolvedValue(
			jsonResponse({
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
			}),
		);
		vi.stubGlobal("fetch", fetchMock);
		const { runSovereignRetention } = await importClient();

		const result = await runSovereignRetention(true);

		const init = fetchMock.mock.calls[0]?.[1] as { body: string };
		expect(JSON.parse(init.body)).toEqual({ dry_run: true });
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

	it("reads the omitempty path and checksum on a live export action", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(
				jsonResponse({
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
				}),
			),
		);
		const { runSovereignRetention } = await importClient();

		const result = await runSovereignRetention(false);

		expect(result.dry_run).toBe(false);
		expect(result.actions[0]?.rows).toBe(51120);
		expect(result.actions[0]?.path).toContain("y2025m12_20260812.jsonl.gz");
	});

	// RunRetention builds retentionRunResponse{DryRun: dryRun} and only appends
	// to Actions, so a run that plans nothing encodes the nil slice as the JSON
	// literal null. The result panel iterates actions unconditionally.
	it("turns a null actions slice into an empty array", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(jsonResponse({ dry_run: true, actions: null })),
		);
		const { runSovereignRetention } = await importClient();

		const result = await runSovereignRetention(true);

		expect(result.actions).toEqual([]);
		expect(result.actions.length).toBe(0);
	});

	// The handler answers 200 with the error text in the body when RunRetention
	// fails its "no valid snapshot" precondition, and Actions is nil there too.
	it("keeps the error text while still exposing an iterable actions array", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(
				jsonResponse({
					dry_run: false,
					actions: null,
					error:
						"no valid snapshot found; create a snapshot before running retention",
				}),
			),
		);
		const { runSovereignRetention } = await importClient();

		const result = await runSovereignRetention(false);

		expect(result.error).toBe(
			"no valid snapshot found; create a snapshot before running retention",
		);
		expect(result.actions).toEqual([]);
	});
});

import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterAll, beforeEach, describe, expect, it, vi } from "vitest";

const { env } = vi.hoisted(() => ({
	env: {} as Record<string, string | undefined>,
}));
vi.mock("$env/dynamic/private", () => ({ env }));

const secretDir = mkdtempSync(join(tmpdir(), "sovereign-admin-"));
const tokenPath = join(secretDir, "sovereign_admin_token");
writeFileSync(tokenPath, "s3cret-admin-token-value-long\n");

afterAll(() => {
	rmSync(secretDir, { recursive: true, force: true });
});

// Fixtures transcribed from the knowledge-sovereign Go source: every /admin/*
// list endpoint wraps its rows in a named envelope and every row carries
// snake_case json tags (handler/{snapshot,storage,retention}_handler.go,
// driver/sovereign_db/{snapshot,retention}.go).
const SNAPSHOT_LATEST = {
	snapshot_id: "550e8400-e29b-41d4-a716-446655440000",
	snapshot_type: "full",
	projection_version: 3,
	projector_build_ref: "sha-9f2c1ab",
	schema_version: "00042",
	snapshot_at: "2026-08-11T02:15:00Z",
	event_seq_boundary: 918442,
	snapshot_data_path:
		"/var/lib/knowledge-sovereign/snapshots/snapshot_20260811_021500",
	items_row_count: 15234,
	items_checksum: "sha256:6f1d0c9a5b2e4d7f8a3c1b0e9d2f4a6c",
	digest_row_count: 30,
	digest_checksum: "sha256:1a2b3c4d5e6f708192a3b4c5d6e7f809",
	recall_row_count: 512,
	recall_checksum: "sha256:9e8d7c6b5a4f3e2d1c0b9a8f7e6d5c4b",
	created_at: "2026-08-11T02:15:07Z",
	status: "valid",
};

const SNAPSHOT_OLDER = {
	...SNAPSHOT_LATEST,
	snapshot_id: "550e8400-e29b-41d4-a716-4466554400ff",
	projection_version: 2,
	snapshot_at: "2026-08-04T02:15:00Z",
	event_seq_boundary: 871003,
	created_at: "2026-08-04T02:15:06Z",
	status: "archived",
};

const STORAGE_STATS_BODY = {
	tables: [
		{
			name: "knowledge_events",
			row_count: 918442,
			total_size: "760 MB",
			table_size: "612 MB",
			index_size: "148 MB",
			total_bytes: 796917760,
			is_partitioned: true,
		},
		{
			name: "knowledge_home_items",
			row_count: 15234,
			total_size: "12 MB",
			table_size: "9 MB",
			index_size: "3 MB",
			total_bytes: 12582912,
			is_partitioned: false,
		},
	],
};

const RETENTION_STATUS_BODY = {
	logs: [
		{
			log_id: "660e8400-e29b-41d4-a716-446655440001",
			run_at: "2026-08-10T10:00:00Z",
			action: "export",
			target_table: "knowledge_events",
			target_partition: "knowledge_events_y2025m11",
			rows_affected: 42000,
			archive_path:
				"/var/lib/knowledge-sovereign/archives/knowledge_events_y2025m11_20260810.jsonl.gz",
			checksum: "sha256:4c3b2a1908f7e6d5c4b3a29180f7e6d5",
			dry_run: false,
			status: "exported",
		},
		// omitempty: archive_path / checksum / error_message are absent on a dry run.
		{
			log_id: "660e8400-e29b-41d4-a716-446655440002",
			run_at: "2026-08-09T10:00:00Z",
			action: "export",
			target_table: "knowledge_user_events",
			target_partition: "knowledge_user_events_y2025m11",
			rows_affected: 0,
			dry_run: true,
			status: "dry_run",
		},
	],
};

const ELIGIBLE_BODY = {
	partitions: [
		{
			table_name: "knowledge_events",
			partition_name: "knowledge_events_y2025m11",
			range_start: "2025-11-01T00:00:00Z",
			range_end: "2025-12-01T00:00:00Z",
			row_count: 42000,
			size_bytes: 52428800,
		},
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
			partition_name: "knowledge_user_events_y2025m11",
			range_start: "2025-11-01T00:00:00Z",
			range_end: "2025-12-01T00:00:00Z",
			row_count: 8100,
			size_bytes: 10485760,
		},
	],
};

const RETENTION_RUN_BODY = {
	dry_run: true,
	actions: [
		{
			action: "export",
			table: "knowledge_events",
			partition: "knowledge_events_y2025m11",
			rows: 0,
			status: "dry_run",
		},
	],
};

const GET_BODIES: Record<string, unknown> = {
	"/admin/storage/stats": STORAGE_STATS_BODY,
	"/admin/snapshots/list": { snapshots: [SNAPSHOT_LATEST, SNAPSHOT_OLDER] },
	"/admin/snapshots/latest": SNAPSHOT_LATEST,
	"/admin/retention/status": RETENTION_STATUS_BODY,
	"/admin/retention/eligible": ELIGIBLE_BODY,
};

function jsonResponse(body: unknown) {
	return {
		ok: true,
		status: 200,
		json: () => Promise.resolve(body),
	};
}

function mockGets(overrides: Record<string, unknown> = {}) {
	const bodies = { ...GET_BODIES, ...overrides };
	const fetchMock = vi.fn((url: string) => {
		const path = new URL(url).pathname;
		if (!(path in bodies)) {
			throw new Error(`unexpected fetch: ${url}`);
		}
		return Promise.resolve(jsonResponse(bodies[path]));
	});
	vi.stubGlobal("fetch", fetchMock);
	return fetchMock;
}

function authHeaderOf(call: unknown[]): string | undefined {
	const init = call[1] as { headers?: Record<string, string> } | undefined;
	return init?.headers?.Authorization;
}

describe("sovereign admin client authorization", () => {
	beforeEach(() => {
		vi.resetModules();
		vi.unstubAllGlobals();
		for (const key of Object.keys(env)) delete env[key];
		env.SOVEREIGN_METRICS_URL = "http://knowledge-sovereign:9501";
	});

	it("sends the admin Bearer token on every /admin/* GET", async () => {
		env.SOVEREIGN_ADMIN_TOKEN_FILE = tokenPath;
		const fetchMock = mockGets();

		const { fetchSovereignAdminSnapshot } = await import("./sovereign-admin");
		await fetchSovereignAdminSnapshot();

		expect(fetchMock).toHaveBeenCalledTimes(5);
		for (const call of fetchMock.mock.calls) {
			expect(authHeaderOf(call)).toBe("Bearer s3cret-admin-token-value-long");
		}
	});

	it("sends the admin Bearer token on the state-changing POSTs", async () => {
		env.SOVEREIGN_ADMIN_TOKEN_FILE = tokenPath;
		const fetchMock = vi.fn((url: string) =>
			Promise.resolve(
				jsonResponse(
					new URL(url).pathname === "/admin/snapshots/create"
						? SNAPSHOT_LATEST
						: RETENTION_RUN_BODY,
				),
			),
		);
		vi.stubGlobal("fetch", fetchMock);

		const { createSovereignSnapshot, runSovereignRetention } = await import(
			"./sovereign-admin"
		);
		await createSovereignSnapshot();
		await runSovereignRetention(true);

		expect(fetchMock).toHaveBeenCalledTimes(2);
		for (const call of fetchMock.mock.calls) {
			expect(authHeaderOf(call)).toBe("Bearer s3cret-admin-token-value-long");
		}
	});

	it("rejects instead of reporting an empty, healthy-looking snapshot when the admin endpoints answer 401", async () => {
		env.SOVEREIGN_ADMIN_TOKEN_FILE = tokenPath;
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue({
				ok: false,
				status: 401,
				json: () => Promise.resolve({}),
			}),
		);

		const { fetchSovereignAdminSnapshot } = await import("./sovereign-admin");

		await expect(fetchSovereignAdminSnapshot()).rejects.toThrow("401");
	});

	// `vite build` imports every server module to analyse the routes, on a
	// machine that holds no runtime secrets, so importing must resolve nothing.
	it("imports without touching the admin config", async () => {
		const { verifySovereignAdminAuth } = await import("./sovereign-admin");

		expect(verifySovereignAdminAuth).toBeTypeOf("function");
	});

	it("fails fast on startup verification when no admin token is configured", async () => {
		const { verifySovereignAdminAuth } = await import("./sovereign-admin");

		expect(verifySovereignAdminAuth).toThrow(/SOVEREIGN_ADMIN_TOKEN/);
	});

	it("refuses to call /admin/* unauthenticated when startup verification was skipped", async () => {
		const fetchMock = mockGets();
		const { fetchSovereignAdminSnapshot } = await import("./sovereign-admin");

		await expect(fetchSovereignAdminSnapshot()).rejects.toThrow(
			/SOVEREIGN_ADMIN_TOKEN/,
		);
		expect(fetchMock).not.toHaveBeenCalled();
	});

	it("names the config key, the token file path and the OS reason when the token file cannot be read", async () => {
		const missingPath = join(secretDir, "does_not_exist");
		env.SOVEREIGN_ADMIN_TOKEN_FILE = missingPath;

		const { verifySovereignAdminAuth } = await import("./sovereign-admin");

		expect(verifySovereignAdminAuth).toThrow("SOVEREIGN_ADMIN_TOKEN_FILE");
		expect(verifySovereignAdminAuth).toThrow(missingPath);
		expect(verifySovereignAdminAuth).toThrow("ENOENT");
	});

	it("runs without a token only when auth is explicitly disabled", async () => {
		env.SOVEREIGN_ADMIN_AUTH = "disabled";
		const fetchMock = mockGets();

		const { fetchSovereignAdminSnapshot } = await import("./sovereign-admin");
		await fetchSovereignAdminSnapshot();

		expect(fetchMock).toHaveBeenCalledTimes(5);
		for (const call of fetchMock.mock.calls) {
			expect(authHeaderOf(call)).toBeUndefined();
		}
	});
});

describe("sovereign admin response parsing", () => {
	beforeEach(() => {
		vi.resetModules();
		vi.unstubAllGlobals();
		for (const key of Object.keys(env)) delete env[key];
		env.SOVEREIGN_METRICS_URL = "http://knowledge-sovereign:9501";
		env.SOVEREIGN_ADMIN_TOKEN_FILE = tokenPath;
	});

	it("unwraps the snapshots envelope and reads the snake_case snapshot fields", async () => {
		mockGets();
		const { fetchSovereignAdminSnapshot } = await import("./sovereign-admin");

		const snapshot = await fetchSovereignAdminSnapshot();

		expect(snapshot.snapshots).toHaveLength(2);
		expect(snapshot.snapshots[0]).toEqual({
			snapshotId: "550e8400-e29b-41d4-a716-446655440000",
			snapshotType: "full",
			projectionVersion: 3,
			projectorBuildRef: "sha-9f2c1ab",
			schemaVersion: "00042",
			snapshotAt: "2026-08-11T02:15:00Z",
			eventSeqBoundary: 918442,
			snapshotDataPath:
				"/var/lib/knowledge-sovereign/snapshots/snapshot_20260811_021500",
			itemsRowCount: 15234,
			itemsChecksum: "sha256:6f1d0c9a5b2e4d7f8a3c1b0e9d2f4a6c",
			digestRowCount: 30,
			digestChecksum: "sha256:1a2b3c4d5e6f708192a3b4c5d6e7f809",
			recallRowCount: 512,
			recallChecksum: "sha256:9e8d7c6b5a4f3e2d1c0b9a8f7e6d5c4b",
			createdAt: "2026-08-11T02:15:07Z",
			status: "valid",
		});
		expect(snapshot.snapshots[1]?.status).toBe("archived");
	});

	it("reads the un-enveloped latest snapshot and keeps null when none exists", async () => {
		mockGets();
		const mod = await import("./sovereign-admin");

		const withLatest = await mod.fetchSovereignAdminSnapshot();
		expect(withLatest.latestSnapshot?.snapshotId).toBe(
			"550e8400-e29b-41d4-a716-446655440000",
		);
		expect(withLatest.latestSnapshot?.eventSeqBoundary).toBe(918442);

		mockGets({ "/admin/snapshots/latest": null });
		const withoutLatest = await mod.fetchSovereignAdminSnapshot();
		expect(withoutLatest.latestSnapshot).toBeNull();
	});

	it("unwraps the tables envelope and exposes the table name the panel keys on", async () => {
		mockGets();
		const { fetchSovereignAdminSnapshot } = await import("./sovereign-admin");

		const snapshot = await fetchSovereignAdminSnapshot();

		expect(snapshot.storageStats).toHaveLength(2);
		expect(snapshot.storageStats[0]).toEqual({
			table_name: "knowledge_events",
			row_count: 918442,
			total_size: "760 MB",
			total_bytes: 796917760,
			is_partitioned: true,
		});
		expect(snapshot.storageStats[1]?.table_name).toBe("knowledge_home_items");
	});

	it("unwraps the logs envelope and reads the snake_case retention log fields", async () => {
		mockGets();
		const { fetchSovereignAdminSnapshot } = await import("./sovereign-admin");

		const snapshot = await fetchSovereignAdminSnapshot();

		expect(snapshot.retentionLogs).toHaveLength(2);
		expect(snapshot.retentionLogs[0]).toEqual({
			logId: "660e8400-e29b-41d4-a716-446655440001",
			runAt: "2026-08-10T10:00:00Z",
			action: "export",
			targetTable: "knowledge_events",
			targetPartition: "knowledge_events_y2025m11",
			rowsAffected: 42000,
			archivePath:
				"/var/lib/knowledge-sovereign/archives/knowledge_events_y2025m11_20260810.jsonl.gz",
			checksum: "sha256:4c3b2a1908f7e6d5c4b3a29180f7e6d5",
			dryRun: false,
			status: "exported",
			errorMessage: "",
		});
		expect(snapshot.retentionLogs[1]?.archivePath).toBe("");
		expect(snapshot.retentionLogs[1]?.dryRun).toBe(true);
	});

	it("groups the flat eligible-partition rows by table", async () => {
		mockGets();
		const { fetchSovereignAdminSnapshot } = await import("./sovereign-admin");

		const snapshot = await fetchSovereignAdminSnapshot();

		expect(snapshot.eligiblePartitions).toEqual([
			{
				table: "knowledge_events",
				eligible: [
					{
						name: "knowledge_events_y2025m11",
						rangeStart: "2025-11-01T00:00:00Z",
						rangeEnd: "2025-12-01T00:00:00Z",
						rowCount: 42000,
						sizeBytes: 52428800,
					},
					{
						name: "knowledge_events_y2025m12",
						rangeStart: "2025-12-01T00:00:00Z",
						rangeEnd: "2026-01-01T00:00:00Z",
						rowCount: 51120,
						sizeBytes: 63963136,
					},
				],
			},
			{
				table: "knowledge_user_events",
				eligible: [
					{
						name: "knowledge_user_events_y2025m11",
						rangeStart: "2025-11-01T00:00:00Z",
						rangeEnd: "2025-12-01T00:00:00Z",
						rowCount: 8100,
						sizeBytes: 10485760,
					},
				],
			},
		]);
	});

	it("reads the created snapshot from the un-enveloped POST response", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(jsonResponse(SNAPSHOT_LATEST)),
		);
		const { createSovereignSnapshot } = await import("./sovereign-admin");

		const created = await createSovereignSnapshot();

		expect(created.snapshotId).toBe("550e8400-e29b-41d4-a716-446655440000");
		expect(created.itemsRowCount).toBe(15234);
		expect(created.status).toBe("valid");
	});

	it("sends dry_run and returns the retention run result as-is", async () => {
		const fetchMock = vi
			.fn()
			.mockResolvedValue(jsonResponse(RETENTION_RUN_BODY));
		vi.stubGlobal("fetch", fetchMock);
		const { runSovereignRetention } = await import("./sovereign-admin");

		const result = await runSovereignRetention(true);

		const init = fetchMock.mock.calls[0]?.[1] as { body: string };
		expect(JSON.parse(init.body)).toEqual({ dry_run: true });
		expect(result.dry_run).toBe(true);
		expect(result.actions[0]?.partition).toBe("knowledge_events_y2025m11");
	});
});

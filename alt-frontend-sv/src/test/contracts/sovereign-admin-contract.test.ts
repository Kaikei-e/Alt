/**
 * Sovereign Admin REST API Contract Tests
 *
 * Validates that TypeScript interfaces match the JSON shapes
 * returned by knowledge-sovereign admin REST endpoints.
 * Go structs without json tags serialize to PascalCase;
 * our server layer normalizes to camelCase.
 */
import { describe, it, expect } from "vitest";
import type {
	TableStorageInfo,
	SnapshotMetadata,
	RetentionLogEntry,
	EligiblePartitionsResult,
	RetentionRunResponse,
	SovereignAdminSnapshot,
	ProjectionAuditData,
} from "$lib/types/sovereign-admin";

/** Raw PascalCase shape from Go (no json tags). */
const RAW_SNAPSHOT_METADATA_PASCAL = {
	SnapshotID: "550e8400-e29b-41d4-a716-446655440000",
	SnapshotType: "full",
	ProjectionVersion: 1,
	ProjectorBuildRef: "abc123",
	SchemaVersion: "00009",
	SnapshotAt: "2026-03-25T12:00:00Z",
	EventSeqBoundary: 786652,
	SnapshotDataPath: "/tmp/snapshots/snapshot_20260325_120000",
	ItemsRowCount: 15000,
	ItemsChecksum: "sha256:abcdef1234567890",
	DigestRowCount: 30,
	DigestChecksum: "sha256:digest1234567890",
	RecallRowCount: 500,
	RecallChecksum: "sha256:recall1234567890",
	CreatedAt: "2026-03-25T12:00:01Z",
	Status: "valid",
};

const RAW_RETENTION_LOG_PASCAL = {
	LogID: "660e8400-e29b-41d4-a716-446655440001",
	RunAt: "2026-03-25T10:00:00Z",
	Action: "export",
	TargetTable: "knowledge_events",
	TargetPartition: "knowledge_events_y2025m11",
	RowsAffected: 42000,
	ArchivePath: "/tmp/archives/knowledge_events_y2025m11_20260325.jsonl.gz",
	Checksum: "sha256:archivechecksum",
	DryRun: false,
	Status: "exported",
	ErrorMessage: "",
};

const RAW_PARTITION_INFO_PASCAL = {
	Name: "knowledge_events_y2025m11",
	RangeStart: "2025-11-01T00:00:00Z",
	RangeEnd: "2025-12-01T00:00:00Z",
	RowCount: 42000,
	SizeBytes: 52428800,
};

describe("Sovereign Admin REST Contract", () => {
	describe("TableStorageInfo (snake_case — has json tags)", () => {
		it("validates shape with snake_case keys", () => {
			const raw = {
				table_name: "knowledge_events",
				row_count: 786652,
				total_size: "760 MB",
				total_bytes: 796917760,
				is_partitioned: true,
			};
			const info: TableStorageInfo = raw;
			expect(info.table_name).toBe("knowledge_events");
			expect(info.row_count).toBe(786652);
			expect(info.total_size).toBe("760 MB");
			expect(info.total_bytes).toBe(796917760);
			expect(info.is_partitioned).toBe(true);
		});

		it("handles array of storage stats", () => {
			const raw: TableStorageInfo[] = [
				{
					table_name: "knowledge_events",
					row_count: 786652,
					total_size: "760 MB",
					total_bytes: 796917760,
					is_partitioned: true,
				},
				{
					table_name: "knowledge_home_items",
					row_count: 15000,
					total_size: "12 MB",
					total_bytes: 12582912,
					is_partitioned: false,
				},
			];
			expect(raw).toHaveLength(2);
			expect(raw[0].is_partitioned).toBe(true);
			expect(raw[1].is_partitioned).toBe(false);
		});
	});

	describe("SnapshotMetadata (PascalCase → normalized camelCase)", () => {
		it("normalizes PascalCase Go response to camelCase", () => {
			const normalized = normalizeSnapshotMetadata(RAW_SNAPSHOT_METADATA_PASCAL);
			expect(normalized.snapshotId).toBe("550e8400-e29b-41d4-a716-446655440000");
			expect(normalized.snapshotType).toBe("full");
			expect(normalized.projectionVersion).toBe(1);
			expect(normalized.eventSeqBoundary).toBe(786652);
			expect(normalized.itemsRowCount).toBe(15000);
			expect(normalized.status).toBe("valid");
		});

		it("validates all required fields are present after normalization", () => {
			const normalized = normalizeSnapshotMetadata(RAW_SNAPSHOT_METADATA_PASCAL);
			const requiredKeys: (keyof SnapshotMetadata)[] = [
				"snapshotId", "snapshotType", "projectionVersion", "projectorBuildRef",
				"schemaVersion", "snapshotAt", "eventSeqBoundary", "snapshotDataPath",
				"itemsRowCount", "itemsChecksum", "digestRowCount", "digestChecksum",
				"recallRowCount", "recallChecksum", "createdAt", "status",
			];
			for (const key of requiredKeys) {
				expect(normalized[key], `missing key: ${key}`).toBeDefined();
			}
		});
	});

	describe("RetentionLogEntry (PascalCase → normalized camelCase)", () => {
		it("normalizes PascalCase Go response to camelCase", () => {
			const normalized = normalizeRetentionLogEntry(RAW_RETENTION_LOG_PASCAL);
			expect(normalized.logId).toBe("660e8400-e29b-41d4-a716-446655440001");
			expect(normalized.action).toBe("export");
			expect(normalized.targetTable).toBe("knowledge_events");
			expect(normalized.rowsAffected).toBe(42000);
			expect(normalized.dryRun).toBe(false);
		});
	});

	describe("EligiblePartitionsResult", () => {
		it("normalizes nested PartitionInfo from PascalCase", () => {
			const raw = {
				table: "knowledge_events",
				eligible: [RAW_PARTITION_INFO_PASCAL],
			};
			const normalized = normalizeEligiblePartitionsResult(raw);
			expect(normalized.table).toBe("knowledge_events");
			expect(normalized.eligible).toHaveLength(1);
			expect(normalized.eligible[0].name).toBe("knowledge_events_y2025m11");
			expect(normalized.eligible[0].sizeBytes).toBe(52428800);
		});
	});

	describe("RetentionRunResponse (snake_case — has json tags)", () => {
		it("validates dry_run response shape", () => {
			const raw: RetentionRunResponse = {
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
			expect(raw.dry_run).toBe(true);
			expect(raw.actions).toHaveLength(1);
			expect(raw.actions[0].status).toBe("dry_run");
		});

		it("validates live run response with path and checksum", () => {
			const raw: RetentionRunResponse = {
				dry_run: false,
				actions: [
					{
						action: "export",
						table: "knowledge_events",
						partition: "knowledge_events_y2025m11",
						rows: 42000,
						path: "/tmp/archives/knowledge_events_y2025m11_20260325.jsonl.gz",
						checksum: "sha256:archivechecksum",
						status: "exported",
					},
				],
			};
			expect(raw.dry_run).toBe(false);
			expect(raw.actions[0].rows).toBe(42000);
			expect(raw.actions[0].path).toBeDefined();
		});
	});

	describe("ProjectionAuditData", () => {
		it("validates audit response shape", () => {
			const audit: ProjectionAuditData = {
				auditId: "550e8400-e29b-41d4-a716-446655440000",
				projectionName: "knowledge_home",
				projectionVersion: "2",
				checkedAt: "2026-03-25T12:00:00Z",
				sampleSize: 100,
				mismatchCount: 2,
				detailsJson: '{"mismatched_items":["article:1","article:2"]}',
			};
			expect(audit.sampleSize).toBe(100);
			expect(audit.mismatchCount).toBe(2);
			expect(JSON.parse(audit.detailsJson)).toHaveProperty("mismatched_items");
		});
	});

	describe("SovereignAdminSnapshot (combined)", () => {
		it("validates combined snapshot shape", () => {
			const snapshot: SovereignAdminSnapshot = {
				storageStats: [],
				snapshots: [],
				latestSnapshot: null,
				retentionLogs: [],
				eligiblePartitions: [],
			};
			expect(snapshot.storageStats).toEqual([]);
			expect(snapshot.latestSnapshot).toBeNull();
		});
	});
});

// --- Normalization functions (to be imported from sovereign-admin.ts server module) ---
// For now, inline implementations that WILL BE replaced by imports once the module exists.

function normalizeSnapshotMetadata(raw: Record<string, unknown>): SnapshotMetadata {
	return {
		snapshotId: raw.SnapshotID as string,
		snapshotType: raw.SnapshotType as string,
		projectionVersion: raw.ProjectionVersion as number,
		projectorBuildRef: raw.ProjectorBuildRef as string,
		schemaVersion: raw.SchemaVersion as string,
		snapshotAt: raw.SnapshotAt as string,
		eventSeqBoundary: raw.EventSeqBoundary as number,
		snapshotDataPath: raw.SnapshotDataPath as string,
		itemsRowCount: raw.ItemsRowCount as number,
		itemsChecksum: raw.ItemsChecksum as string,
		digestRowCount: raw.DigestRowCount as number,
		digestChecksum: raw.DigestChecksum as string,
		recallRowCount: raw.RecallRowCount as number,
		recallChecksum: raw.RecallChecksum as string,
		createdAt: raw.CreatedAt as string,
		status: raw.Status as string,
	};
}

function normalizeRetentionLogEntry(raw: Record<string, unknown>): RetentionLogEntry {
	return {
		logId: raw.LogID as string,
		runAt: raw.RunAt as string,
		action: raw.Action as string,
		targetTable: raw.TargetTable as string,
		targetPartition: raw.TargetPartition as string,
		rowsAffected: raw.RowsAffected as number,
		archivePath: raw.ArchivePath as string,
		checksum: raw.Checksum as string,
		dryRun: raw.DryRun as boolean,
		status: raw.Status as string,
		errorMessage: raw.ErrorMessage as string,
	};
}

function normalizeEligiblePartitionsResult(raw: {
	table: string;
	eligible: Record<string, unknown>[];
}): EligiblePartitionsResult {
	return {
		table: raw.table,
		eligible: raw.eligible.map((p) => ({
			name: p.Name as string,
			rangeStart: p.RangeStart as string,
			rangeEnd: p.RangeEnd as string,
			rowCount: p.RowCount as number,
			sizeBytes: p.SizeBytes as number,
		})),
	};
}

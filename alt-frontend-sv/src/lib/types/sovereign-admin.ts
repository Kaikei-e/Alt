/**
 * TypeScript interfaces for knowledge-sovereign admin REST API responses.
 *
 * Group A endpoints: plain REST on knowledge-sovereign metrics port (:9501).
 * All PascalCase Go fields are normalized to camelCase by the server layer.
 */

/** Storage stats for a knowledge table (from GET /admin/storage/stats). */
export interface TableStorageInfo {
	table_name: string;
	row_count: number;
	total_size: string;
	total_bytes: number;
	is_partitioned: boolean;
}

/** Partition info for a partitioned table. */
export interface PartitionInfo {
	name: string;
	rangeStart: string;
	rangeEnd: string;
	rowCount: number;
	sizeBytes: number;
}

/** Eligible partitions for a table (from GET /admin/retention/eligible). */
export interface EligiblePartitionsResult {
	table: string;
	eligible: PartitionInfo[];
}

/** Retention log entry (from GET /admin/retention/status). */
export interface RetentionLogEntry {
	logId: string;
	runAt: string;
	action: string;
	targetTable: string;
	targetPartition: string;
	rowsAffected: number;
	archivePath: string;
	checksum: string;
	dryRun: boolean;
	status: string;
	errorMessage: string;
}

/** Individual retention action in a run result. */
export interface RetentionAction {
	action: string;
	table: string;
	partition: string;
	rows: number;
	path?: string;
	checksum?: string;
	status: string;
}

/** Result of POST /admin/retention/run. */
export interface RetentionRunResponse {
	dry_run: boolean;
	actions: RetentionAction[];
	error?: string;
}

/** Projection snapshot metadata (from GET /admin/snapshots/*). */
export interface SnapshotMetadata {
	snapshotId: string;
	snapshotType: string;
	projectionVersion: number;
	projectorBuildRef: string;
	schemaVersion: string;
	snapshotAt: string;
	eventSeqBoundary: number;
	snapshotDataPath: string;
	itemsRowCount: number;
	itemsChecksum: string;
	digestRowCount: number;
	digestChecksum: string;
	recallRowCount: number;
	recallChecksum: string;
	createdAt: string;
	status: string;
}

/** Combined snapshot for sovereign admin polling. */
export interface SovereignAdminSnapshot {
	storageStats: TableStorageInfo[];
	snapshots: SnapshotMetadata[];
	latestSnapshot: SnapshotMetadata | null;
	retentionLogs: RetentionLogEntry[];
	eligiblePartitions: EligiblePartitionsResult[];
}

/** Projection audit result (from RunProjectionAudit Connect-RPC). */
export interface ProjectionAuditData {
	auditId: string;
	projectionName: string;
	projectionVersion: string;
	checkedAt: string;
	sampleSize: number;
	mismatchCount: number;
	detailsJson: string;
}

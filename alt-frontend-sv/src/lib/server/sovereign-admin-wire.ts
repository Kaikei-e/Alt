/**
 * Wire shapes of knowledge-sovereign's /admin/* responses and their translation
 * into the view types.
 *
 * The list endpoints answer with a named envelope ("tables", "snapshots",
 * "logs", "partitions") around snake_case rows; the single-object endpoints
 * (snapshots/latest, snapshots/create) answer with a bare snake_case object.
 *
 * Kept apart from the client that fetches them because that client reads
 * $env/dynamic/private, which only vite can resolve: contract tests run under
 * the bun test runner and can import this module but not that one.
 */

import type {
	EligiblePartitionsResult,
	PartitionInfo,
	RetentionAction,
	RetentionLogEntry,
	RetentionRunResponse,
	SnapshotMetadata,
	SovereignAdminSnapshot,
	TableStorageInfo,
} from "$lib/types/sovereign-admin";

export type RawRow = Record<string, unknown>;

/** The five bodies behind one SovereignAdminSnapshot, as they arrive. */
export interface SovereignAdminWire {
	storage: { tables: RawRow[] };
	snapshotList: { snapshots: RawRow[] };
	latestSnapshot: RawRow | null;
	retentionStatus: { logs: RawRow[] };
	eligible: { partitions: RawRow[] };
}

export type RawRetentionRun = Omit<RetentionRunResponse, "actions"> & {
	actions: RetentionAction[] | null;
};

// omitempty json tags drop the key entirely; the view types declare a string.
function optionalString(value: unknown): string {
	return (value as string | undefined) ?? "";
}

export function normalizeTableStorageInfo(raw: RawRow): TableStorageInfo {
	return {
		table_name: raw.name as string,
		row_count: raw.row_count as number,
		total_size: raw.total_size as string,
		total_bytes: raw.total_bytes as number,
		is_partitioned: raw.is_partitioned as boolean,
	};
}

export function normalizeSnapshotMetadata(raw: RawRow): SnapshotMetadata {
	return {
		snapshotId: raw.snapshot_id as string,
		snapshotType: raw.snapshot_type as string,
		projectionVersion: raw.projection_version as number,
		projectorBuildRef: raw.projector_build_ref as string,
		schemaVersion: raw.schema_version as string,
		snapshotAt: raw.snapshot_at as string,
		eventSeqBoundary: raw.event_seq_boundary as number,
		snapshotDataPath: raw.snapshot_data_path as string,
		itemsRowCount: raw.items_row_count as number,
		itemsChecksum: raw.items_checksum as string,
		digestRowCount: raw.digest_row_count as number,
		digestChecksum: raw.digest_checksum as string,
		recallRowCount: raw.recall_row_count as number,
		recallChecksum: raw.recall_checksum as string,
		createdAt: raw.created_at as string,
		status: raw.status as string,
	};
}

export function normalizeRetentionLogEntry(raw: RawRow): RetentionLogEntry {
	return {
		logId: raw.log_id as string,
		runAt: raw.run_at as string,
		action: raw.action as string,
		targetTable: raw.target_table as string,
		targetPartition: raw.target_partition as string,
		rowsAffected: raw.rows_affected as number,
		archivePath: optionalString(raw.archive_path),
		checksum: optionalString(raw.checksum),
		dryRun: raw.dry_run as boolean,
		status: raw.status as string,
		errorMessage: optionalString(raw.error_message),
	};
}

// /admin/retention/eligible answers a single flat array of table-tagged rows;
// the panel renders one section per table, so regroup on the way in.
export function groupEligiblePartitions(
	rows: RawRow[],
): EligiblePartitionsResult[] {
	const byTable = new Map<string, EligiblePartitionsResult>();
	for (const row of rows) {
		const table = row.table_name as string;
		const partition: PartitionInfo = {
			name: row.partition_name as string,
			rangeStart: row.range_start as string,
			rangeEnd: row.range_end as string,
			rowCount: row.row_count as number,
			sizeBytes: row.size_bytes as number,
		};
		const group = byTable.get(table);
		if (group) {
			group.eligible.push(partition);
		} else {
			byTable.set(table, { table, eligible: [partition] });
		}
	}
	return [...byTable.values()];
}

export function normalizeSovereignAdminSnapshot(
	wire: SovereignAdminWire,
): SovereignAdminSnapshot {
	return {
		storageStats: wire.storage.tables.map(normalizeTableStorageInfo),
		snapshots: wire.snapshotList.snapshots.map(normalizeSnapshotMetadata),
		latestSnapshot: wire.latestSnapshot
			? normalizeSnapshotMetadata(wire.latestSnapshot)
			: null,
		retentionLogs: wire.retentionStatus.logs.map(normalizeRetentionLogEntry),
		eligiblePartitions: groupEligiblePartitions(wire.eligible.partitions),
	};
}

export function normalizeRetentionRun(
	raw: RawRetentionRun,
): RetentionRunResponse {
	// The result panel reads .length on actions unconditionally, and this is an
	// external boundary: a nil Go slice would encode as null, not [].
	return { ...raw, actions: raw.actions ?? [] };
}

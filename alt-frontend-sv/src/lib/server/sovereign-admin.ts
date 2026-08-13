/**
 * Server-side client for knowledge-sovereign admin REST endpoints.
 *
 * Calls knowledge-sovereign metrics port (:9501) directly from SvelteKit server.
 * The list endpoints answer with a named envelope ("tables", "snapshots",
 * "logs", "partitions") around snake_case rows; the single-object endpoints
 * (snapshots/latest, snapshots/create) answer with a bare snake_case object.
 * Both are unwrapped and renamed to the camelCase view types here.
 */

import { readFileSync } from "node:fs";
import { env } from "$env/dynamic/private";
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

const SOVEREIGN_METRICS_URL =
	env.SOVEREIGN_METRICS_URL || "http://knowledge-sovereign:9501";

// knowledge-sovereign Bearer-gates every /admin/* route on its metrics port and
// opens it only for an explicit ADMIN_AUTH=disabled, so this caller mirrors that
// switch rather than reading an absent token as "no token needed".
function loadAdminToken(): string | null {
	if (env.SOVEREIGN_ADMIN_AUTH === "disabled") {
		console.warn(
			"sovereign_admin_auth_disabled: SOVEREIGN_ADMIN_AUTH=disabled was set explicitly; /admin/* calls carry no Bearer token",
		);
		return null;
	}

	const tokenFile = env.SOVEREIGN_ADMIN_TOKEN_FILE;
	if (tokenFile) {
		let contents: string;
		try {
			contents = readFileSync(tokenFile, "utf8");
		} catch (cause) {
			// This runs at server startup, where a bare fs error reaches the
			// operator only as a container that exited: name the config key, the
			// path and the OS reason so the log line alone is diagnosable.
			throw new Error(
				`SOVEREIGN_ADMIN_TOKEN_FILE (${tokenFile}) could not be read: ${
					cause instanceof Error ? cause.message : String(cause)
				}`,
				{ cause },
			);
		}
		const token = contents.trim();
		if (!token) {
			throw new Error(`SOVEREIGN_ADMIN_TOKEN_FILE (${tokenFile}) is empty`);
		}
		console.info("sovereign_admin_auth_enabled");
		return token;
	}

	const token = env.SOVEREIGN_ADMIN_TOKEN?.trim();
	if (token) {
		console.info("sovereign_admin_auth_enabled");
		return token;
	}

	throw new Error(
		"SOVEREIGN_ADMIN_TOKEN_FILE or SOVEREIGN_ADMIN_TOKEN is required; set SOVEREIGN_ADMIN_AUTH=disabled to call knowledge-sovereign /admin/* without a token",
	);
}

let resolved: { token: string | null } | undefined;

// Resolved on first use, never at module load: `vite build` imports every server
// module to analyse the routes, and the machine building the image holds no
// runtime secrets. The server hook below moves that first use to process start.
function adminToken(): string | null {
	resolved ??= { token: loadAdminToken() };
	return resolved.token;
}

// Called from the `init` server hook so that a missing token kills the process
// at startup instead of surfacing as 401s the first time someone opens the panel.
export function verifySovereignAdminAuth(): void {
	adminToken();
}

function adminHeaders(
	extra?: Record<string, string>,
): Record<string, string> | undefined {
	const token = adminToken();
	if (!token) {
		return extra;
	}
	return { ...extra, Authorization: `Bearer ${token}` };
}

async function fetchJSON<T>(url: string): Promise<T> {
	const response = await fetch(url, { headers: adminHeaders() });
	if (!response.ok) {
		throw new Error(`Sovereign API error: ${response.status} ${url}`);
	}
	return response.json() as Promise<T>;
}

// --- snake_case → camelCase normalizers ---

type RawRow = Record<string, unknown>;

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

// --- Public API ---

export async function fetchSovereignAdminSnapshot(): Promise<SovereignAdminSnapshot> {
	const [
		storageStats,
		snapshotList,
		rawLatestSnapshot,
		retentionStatus,
		eligible,
	] = await Promise.all([
		fetchJSON<{ tables: RawRow[] }>(
			`${SOVEREIGN_METRICS_URL}/admin/storage/stats`,
		),
		fetchJSON<{ snapshots: RawRow[] }>(
			`${SOVEREIGN_METRICS_URL}/admin/snapshots/list`,
		),
		fetchJSON<RawRow | null>(`${SOVEREIGN_METRICS_URL}/admin/snapshots/latest`),
		fetchJSON<{ logs: RawRow[] }>(
			`${SOVEREIGN_METRICS_URL}/admin/retention/status`,
		),
		fetchJSON<{ partitions: RawRow[] }>(
			`${SOVEREIGN_METRICS_URL}/admin/retention/eligible`,
		),
	]);

	return {
		storageStats: storageStats.tables.map(normalizeTableStorageInfo),
		snapshots: snapshotList.snapshots.map(normalizeSnapshotMetadata),
		latestSnapshot: rawLatestSnapshot
			? normalizeSnapshotMetadata(rawLatestSnapshot)
			: null,
		retentionLogs: retentionStatus.logs.map(normalizeRetentionLogEntry),
		eligiblePartitions: groupEligiblePartitions(eligible.partitions),
	};
}

export async function createSovereignSnapshot(): Promise<SnapshotMetadata> {
	const response = await fetch(
		`${SOVEREIGN_METRICS_URL}/admin/snapshots/create`,
		{ method: "POST", headers: adminHeaders() },
	);
	if (!response.ok) {
		throw new Error(`Failed to create snapshot: ${response.status}`);
	}
	const raw = (await response.json()) as RawRow;
	return normalizeSnapshotMetadata(raw);
}

export async function runSovereignRetention(
	dryRun: boolean,
): Promise<RetentionRunResponse> {
	const response = await fetch(`${SOVEREIGN_METRICS_URL}/admin/retention/run`, {
		method: "POST",
		headers: adminHeaders({ "Content-Type": "application/json" }),
		body: JSON.stringify({ dry_run: dryRun }),
	});
	if (!response.ok) {
		throw new Error(`Failed to run retention: ${response.status}`);
	}
	// A run that plans no action leaves Go's Actions slice nil, which encodes as
	// null, not []. The result panel reads .length on it unconditionally.
	const raw = (await response.json()) as Omit<
		RetentionRunResponse,
		"actions"
	> & { actions: RetentionAction[] | null };
	return { ...raw, actions: raw.actions ?? [] };
}

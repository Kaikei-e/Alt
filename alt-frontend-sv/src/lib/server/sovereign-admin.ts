/**
 * Server-side client for knowledge-sovereign admin REST endpoints.
 *
 * Calls knowledge-sovereign metrics port (:9501) directly from SvelteKit server.
 * PascalCase Go responses (structs without json tags) are normalized to camelCase.
 */

import { env } from "$env/dynamic/private";
import type {
	TableStorageInfo,
	SnapshotMetadata,
	RetentionLogEntry,
	EligiblePartitionsResult,
	RetentionRunResponse,
	SovereignAdminSnapshot,
} from "$lib/types/sovereign-admin";

const SOVEREIGN_METRICS_URL =
	env.SOVEREIGN_METRICS_URL || "http://knowledge-sovereign:9501";

async function fetchJSON<T>(url: string): Promise<T> {
	const response = await fetch(url);
	if (!response.ok) {
		throw new Error(`Sovereign API error: ${response.status} ${url}`);
	}
	return response.json() as Promise<T>;
}

// --- PascalCase → camelCase normalizers ---

export function normalizeSnapshotMetadata(
	raw: Record<string, unknown>,
): SnapshotMetadata {
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

export function normalizeRetentionLogEntry(
	raw: Record<string, unknown>,
): RetentionLogEntry {
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

export function normalizePartitionInfo(raw: Record<string, unknown>) {
	return {
		name: raw.Name as string,
		rangeStart: raw.RangeStart as string,
		rangeEnd: raw.RangeEnd as string,
		rowCount: raw.RowCount as number,
		sizeBytes: raw.SizeBytes as number,
	};
}

export function normalizeEligiblePartitionsResult(
	raw: Record<string, unknown>,
): EligiblePartitionsResult {
	return {
		table: raw.table as string,
		eligible: ((raw.eligible as Record<string, unknown>[]) ?? []).map(
			normalizePartitionInfo,
		),
	};
}

// --- Public API ---

export async function fetchSovereignAdminSnapshot(): Promise<SovereignAdminSnapshot> {
	const [
		storageStats,
		rawSnapshots,
		rawLatestSnapshot,
		rawRetentionLogs,
		rawEligiblePartitions,
	] = await Promise.all([
		fetchJSON<TableStorageInfo[]>(
			`${SOVEREIGN_METRICS_URL}/admin/storage/stats`,
		).catch(() => []),
		fetchJSON<Record<string, unknown>[]>(
			`${SOVEREIGN_METRICS_URL}/admin/snapshots/list`,
		).catch(() => []),
		fetchJSON<Record<string, unknown> | null>(
			`${SOVEREIGN_METRICS_URL}/admin/snapshots/latest`,
		).catch(() => null),
		fetchJSON<Record<string, unknown>[]>(
			`${SOVEREIGN_METRICS_URL}/admin/retention/status`,
		).catch(() => []),
		fetchJSON<Record<string, unknown>[]>(
			`${SOVEREIGN_METRICS_URL}/admin/retention/eligible`,
		).catch(() => []),
	]);

	return {
		storageStats,
		snapshots: rawSnapshots.map(normalizeSnapshotMetadata),
		latestSnapshot: rawLatestSnapshot
			? normalizeSnapshotMetadata(rawLatestSnapshot)
			: null,
		retentionLogs: rawRetentionLogs.map(normalizeRetentionLogEntry),
		eligiblePartitions: rawEligiblePartitions.map(
			normalizeEligiblePartitionsResult,
		),
	};
}

export async function createSovereignSnapshot(): Promise<SnapshotMetadata> {
	const response = await fetch(
		`${SOVEREIGN_METRICS_URL}/admin/snapshots/create`,
		{ method: "POST" },
	);
	if (!response.ok) {
		throw new Error(`Failed to create snapshot: ${response.status}`);
	}
	const raw = (await response.json()) as Record<string, unknown>;
	return normalizeSnapshotMetadata(raw);
}

export async function runSovereignRetention(
	dryRun: boolean,
): Promise<RetentionRunResponse> {
	const response = await fetch(`${SOVEREIGN_METRICS_URL}/admin/retention/run`, {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		body: JSON.stringify({ dry_run: dryRun }),
	});
	if (!response.ok) {
		throw new Error(`Failed to run retention: ${response.status}`);
	}
	return response.json() as Promise<RetentionRunResponse>;
}

/**
 * Mirrors the Go handler's response shape
 * (knowledge-sovereign/app/handler/reproject_knowledge_loop_handler.go).
 * Upstream changes need conscious updates here.
 */
export interface KnowledgeLoopReprojectResult {
	ok: boolean;
	entries_truncated: number;
	session_state_truncated: number;
	surfaces_truncated: number;
	checkpoint_reset: boolean;
	projector_will_run_on_tick: string;
	error?: string;
}

/**
 * Operator-facing status snapshot for the Knowledge Loop projector. Mirrors
 * knowledgeLoopReprojectStatus on the Go side. Lets the admin UI surface
 * "current code is at WhyMappingVersion N; projector caught up to event_seq
 * M" so the operator knows whether a reproject is warranted.
 */
export interface KnowledgeLoopReprojectStatus {
	why_mapping_version: number;
	last_event_seq: number;
	projector_name: string;
	error?: string;
}

export async function fetchKnowledgeLoopReprojectStatus(): Promise<KnowledgeLoopReprojectStatus> {
	const response = await fetch(
		`${SOVEREIGN_METRICS_URL}/admin/knowledge-loop/reproject/status`,
	);
	if (!response.ok) {
		throw new Error(
			`Knowledge Loop reproject status failed (${response.status})`,
		);
	}
	return (await response.json()) as KnowledgeLoopReprojectStatus;
}

/**
 * Triggers a full Knowledge Loop reproject on knowledge-sovereign. Runbook:
 * docs/runbooks/knowledge-loop-reproject.md.
 *
 * Destructive — TRUNCATEs three projection tables and resets the projector
 * checkpoint. The caller (SvelteKit +server.ts) MUST enforce admin auth
 * before invoking. Idempotent: safe to call again after a previous success.
 */
export async function triggerKnowledgeLoopReproject(): Promise<KnowledgeLoopReprojectResult> {
	const response = await fetch(
		`${SOVEREIGN_METRICS_URL}/admin/knowledge-loop/reproject`,
		{ method: "POST" },
	);
	const body = (await response.json()) as KnowledgeLoopReprojectResult;
	if (!response.ok) {
		throw new Error(
			`Knowledge Loop reproject failed (${response.status}): ${body.error ?? "unknown"}`,
		);
	}
	return body;
}

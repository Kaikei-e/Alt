/**
 * AcolyteService client — REST wrapper until proto codegen is complete.
 *
 * All calls go through the SvelteKit /api/v2 proxy → BFF → acolyte-orchestrator.
 * When TypeScript proto generation is ready, replace with createClient(AcolyteService, ...).
 */

const BASE = "/api/v2";

async function rpc<T>(method: string, body: Record<string, unknown> = {}): Promise<T> {
	const resp = await fetch(`${BASE}/alt.acolyte.v1.AcolyteService/${method}`, {
		method: "POST",
		headers: { "Content-Type": "application/json" },
		credentials: "include",
		body: JSON.stringify(body),
	});
	if (!resp.ok) {
		const err = await resp.json().catch(() => ({ message: resp.statusText }));
		throw new Error(err.message ?? `RPC ${method} failed: ${resp.status}`);
	}
	return resp.json();
}

// --- Types ---

export interface AcolyteReport {
	reportId: string;
	title: string;
	reportType: string;
	currentVersion: number;
	latestSuccessfulRunId?: string;
	createdAt: string;
}

export interface AcolyteReportSummary {
	reportId: string;
	title: string;
	reportType: string;
	currentVersion: number;
	latestRunStatus: string;
	createdAt: string;
}

export interface AcolyteSection {
	sectionKey: string;
	currentVersion: number;
	displayOrder: number;
	body: string;
	citationsJson: string;
}

export interface AcolyteChangeItem {
	fieldName: string;
	changeKind: string;
	oldFingerprint: string;
	newFingerprint: string;
}

export interface AcolyteVersionSummary {
	versionNo: number;
	changeReason: string;
	createdAt: string;
	changeItems: AcolyteChangeItem[];
}

export interface AcolyteRun {
	runId: string;
	reportId: string;
	targetVersionNo: number;
	runStatus: string;
	startedAt?: string;
	finishedAt?: string;
	failureCode?: string;
	failureMessage?: string;
}

// --- API calls ---

export async function createReport(
	title: string,
	reportType: string,
	scope?: Record<string, string>,
): Promise<{ reportId: string }> {
	return rpc("CreateReport", { title, reportType, scope: scope ?? {} });
}

export async function getReport(
	reportId: string,
): Promise<{ report: AcolyteReport; sections: AcolyteSection[] }> {
	return rpc("GetReport", { reportId });
}

export async function listReports(
	cursor?: string,
	limit = 20,
): Promise<{
	reports: AcolyteReportSummary[];
	nextCursor: string;
	hasMore: boolean;
}> {
	return rpc("ListReports", { cursor, limit });
}

export async function listReportVersions(
	reportId: string,
	cursor?: string,
	limit = 20,
): Promise<{
	versions: AcolyteVersionSummary[];
	nextCursor: string;
	hasMore: boolean;
}> {
	return rpc("ListReportVersions", { reportId, cursor, limit });
}

export async function startReportRun(
	reportId: string,
): Promise<{ runId: string }> {
	return rpc("StartReportRun", { reportId });
}

export async function getRunStatus(
	runId: string,
): Promise<{ run: AcolyteRun }> {
	return rpc("GetRunStatus", { runId });
}

export async function rerunSection(
	reportId: string,
	sectionKey: string,
): Promise<{ runId: string }> {
	return rpc("RerunSection", { reportId, sectionKey });
}

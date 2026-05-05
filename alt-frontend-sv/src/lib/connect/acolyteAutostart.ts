/**
 * Compose CreateReport → StartReportRun → goto into a single user-visible
 * action so a freshly created Acolyte report immediately enters the
 * generation pipeline. Without this, a report sits at version 0 with no
 * job in `report_jobs` until the user clicks Generate manually — which
 * users routinely miss, leaving reports "pending" forever.
 *
 * StartReportRun failure is non-fatal: the report row already exists, so
 * we still navigate (with `?autostart_failed=1`) and let the detail page
 * surface the error. The user can retry via the existing Generate button.
 */

export interface AutostartDeps {
	createReport: (
		title: string,
		reportType: string,
		scope: Record<string, string>,
	) => Promise<{ reportId: string }>;
	startReportRun: (reportId: string) => Promise<{ runId: string }>;
	goto: (url: string) => Promise<void>;
}

export interface AutostartOutcome {
	ok: boolean;
	reportId?: string;
	runId?: string;
	error?: string;
}

function errorMessage(e: unknown, fallback: string): string {
	if (e instanceof Error) return e.message || fallback;
	return fallback;
}

export async function createAndAutostart(
	deps: AutostartDeps,
	title: string,
	reportType: string,
	scope: Record<string, string>,
): Promise<AutostartOutcome> {
	let reportId: string;
	try {
		const created = await deps.createReport(title, reportType, scope);
		reportId = created.reportId;
	} catch (e) {
		return { ok: false, error: errorMessage(e, "Failed to create report") };
	}

	try {
		const { runId } = await deps.startReportRun(reportId);
		await deps.goto(`/acolyte/reports/${reportId}?run=${runId}`);
		return { ok: true, reportId, runId };
	} catch (e) {
		await deps.goto(`/acolyte/reports/${reportId}?autostart_failed=1`);
		return {
			ok: false,
			reportId,
			error: errorMessage(e, "Failed to start generation"),
		};
	}
}

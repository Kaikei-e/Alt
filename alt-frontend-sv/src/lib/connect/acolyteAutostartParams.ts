/**
 * URL → action resolver for the Acolyte report detail page.
 *
 *   ?run=<runId>            → resume polling that run on mount
 *   ?autostart_failed=1     → show "auto-start failed, click Generate" error
 *   (none)                  → idle; existing manual Generate flow
 *
 * `?run=` wins when both are present so a successful retry overrides a
 * stale failure marker left in the URL.
 */

export type AutostartIntent =
	| { kind: "resume"; runId: string }
	| { kind: "autostart-failed" }
	| { kind: "none" };

export function resolveAutostartIntent(
	params: URLSearchParams,
): AutostartIntent {
	const runId = params.get("run");
	if (runId) return { kind: "resume", runId };
	if (params.get("autostart_failed") === "1")
		return { kind: "autostart-failed" };
	return { kind: "none" };
}

/**
 * Combine the server-supplied active run (from `GetReport.active_run`) with
 * the URL-derived intent so the detail page can resume polling on mount.
 *
 * Backend wins: a pending/running run from the server is the source of
 * truth — the URL `?run=` query param is a hint from the prior /new
 * navigation and may be stale (e.g. an old run already terminated). When
 * the backend reports an active run, we use that runId. Otherwise we fall
 * back to the URL hint or the autostart-failed marker.
 */

export interface ServerActiveRun {
	runId: string;
	runStatus: string;
}

export function resolveResumeIntent(
	params: URLSearchParams,
	activeRun: ServerActiveRun | undefined,
): AutostartIntent {
	if (activeRun && isInFlight(activeRun.runStatus)) {
		return { kind: "resume", runId: activeRun.runId };
	}
	return resolveAutostartIntent(params);
}

function isInFlight(status: string): boolean {
	return status === "pending" || status === "running";
}

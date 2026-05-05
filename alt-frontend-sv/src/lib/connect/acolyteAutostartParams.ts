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

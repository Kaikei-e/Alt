/**
 * Run-status pill model — pure functions + label/color mapping.
 *
 * Separated from the Svelte component so derivation can be unit-tested
 * without DOM/testing-library overhead and so the component itself stays
 * stateless and props-driven (per bp-svelte).
 */

/** Raw backend run status as delivered by AcolyteService.GetRunStatus. */
export type RunStatus =
	| "pending"
	| "running"
	| "succeeded"
	| "failed"
	| "cancelled";

/**
 * Display-layer kind. Driven from runStatus + pendingUpdate + whether the
 * report already has any version. The pill renders labels/colors from this.
 */
export type RunStatusKind =
	| "idle"
	| "ready"
	| "generating"
	| "completed"
	| "failed"
	| "cancelled";

export interface DeriveRunStatusKindInput {
	runStatus: RunStatus | null;
	pendingUpdate: boolean;
	/** `report.currentVersion`, or 0 before the first successful run. */
	currentVersion: number;
}

/**
 * Map the active run state and the pending-refresh flag into a stable
 * display kind. Precedence: generating > failed > cancelled >
 * completed (from pendingUpdate or succeeded) > ready > idle. "generating"
 * wins over a stale `pendingUpdate` flag so a re-run started right after a
 * completion shows the new run, not the old acknowledgement.
 */
export function deriveRunStatusKind(
	input: DeriveRunStatusKindInput,
): RunStatusKind {
	const { runStatus, pendingUpdate, currentVersion } = input;

	if (runStatus === "pending" || runStatus === "running") {
		return "generating";
	}
	if (runStatus === "failed") {
		return "failed";
	}
	if (runStatus === "cancelled") {
		return "cancelled";
	}
	if (pendingUpdate || runStatus === "succeeded") {
		return "completed";
	}
	if (currentVersion > 0) {
		return "ready";
	}
	return "idle";
}

export const RUN_STATUS_LABELS: Record<RunStatusKind, string> = {
	idle: "Idle",
	ready: "Ready",
	generating: "Generating",
	completed: "Updated",
	failed: "Failed",
	cancelled: "Cancelled",
};

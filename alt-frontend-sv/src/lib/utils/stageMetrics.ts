import type {
	StatusTransition,
	PipelineStage,
	JobStatus,
} from "$lib/schema/dashboard";
import { PIPELINE_STAGES } from "$lib/schema/dashboard";

/**
 * Represents the duration metrics for a single pipeline stage
 */
export interface StageDuration {
	stage: PipelineStage;
	durationSecs: number;
	startedAt: string | null;
	completedAt: string | null;
	status: "completed" | "running" | "pending" | "skipped";
}

/**
 * Represents the complete metrics for a job
 */
export interface JobMetrics {
	totalDurationSecs: number;
	stageDurations: StageDuration[];
	avgDurationSecs: number | null;
	performanceRatio: number | null; // < 1 = faster than avg, > 1 = slower
}

/**
 * Calculate duration in seconds between two ISO timestamps
 */
export function calculateDurationSecs(
	startIso: string,
	endIso: string,
): number {
	const start = new Date(startIso).getTime();
	const end = new Date(endIso).getTime();
	return Math.max(0, Math.round((end - start) / 1000));
}

/**
 * Extract stage durations from status history transitions
 *
 * The status_history contains transitions with stages. We infer duration by
 * finding the time difference between when a stage started running
 * and when it completed (or when the next stage started).
 */
export function calculateStageDurations(
	statusHistory: StatusTransition[],
	jobKickedAt: string,
	jobStatus: JobStatus,
): StageDuration[] {
	if (!statusHistory || statusHistory.length === 0) {
		return PIPELINE_STAGES.map((stage) => ({
			stage,
			durationSecs: 0,
			startedAt: null,
			completedAt: null,
			status: "pending" as const,
		}));
	}

	// Sort transitions by time
	const sortedTransitions = [...statusHistory].sort(
		(a, b) =>
			new Date(a.transitioned_at).getTime() -
			new Date(b.transitioned_at).getTime(),
	);

	// Map to track stage timing: stage -> { start, end }
	const stageTiming: Map<
		string,
		{ start: string | null; end: string | null; status: JobStatus }
	> = new Map();

	// Process transitions to extract timing
	for (let i = 0; i < sortedTransitions.length; i++) {
		const transition = sortedTransitions[i];
		const stage = transition.stage;

		if (!stage) continue;

		const existing = stageTiming.get(stage);

		if (transition.status === "running") {
			// Stage started
			if (!existing) {
				stageTiming.set(stage, {
					start: transition.transitioned_at,
					end: null,
					status: "running",
				});
			} else if (!existing.start) {
				existing.start = transition.transitioned_at;
				existing.status = "running";
			}
		} else if (
			transition.status === "completed" ||
			transition.status === "failed"
		) {
			// Stage ended
			if (existing) {
				existing.end = transition.transitioned_at;
				existing.status = transition.status;
			} else {
				stageTiming.set(stage, {
					start: null,
					end: transition.transitioned_at,
					status: transition.status,
				});
			}
		}
	}

	// Infer completion for stages that don't have explicit end time
	// by using the start time of the next stage
	const stageOrder = PIPELINE_STAGES;
	for (let i = 0; i < stageOrder.length - 1; i++) {
		const currentStage = stageOrder[i];
		const nextStage = stageOrder[i + 1];
		const current = stageTiming.get(currentStage);
		const next = stageTiming.get(nextStage);

		if (current && current.start && !current.end && next && next.start) {
			current.end = next.start;
			if (current.status === "running") {
				current.status = "completed";
			}
		}
	}

	// When job is completed, infer timing for stages that have no Status History entries
	// This handles stages like "evidence" that may not record status transitions
	if (jobStatus === "completed") {
		for (let i = 0; i < stageOrder.length; i++) {
			const stage = stageOrder[i];
			const timing = stageTiming.get(stage);

			// Skip stages that already have complete timing data (both start and end)
			if (timing && timing.start && timing.end) {
				continue;
			}

			// Infer timing from surrounding stages
			let inferredStart: string | null = timing?.start ?? null;
			let inferredEnd: string | null = timing?.end ?? null;

			// Use previous stage's end time as start
			if (!inferredStart && i > 0) {
				const prevTiming = stageTiming.get(stageOrder[i - 1]);
				if (prevTiming?.end) {
					inferredStart = prevTiming.end;
				}
			}

			// Use next stage's start time as end
			if (!inferredEnd && i < stageOrder.length - 1) {
				const nextTiming = stageTiming.get(stageOrder[i + 1]);
				if (nextTiming?.start) {
					inferredEnd = nextTiming.start;
				}
			}

			// For stages with no timing at all (like evidence), use next stage's start for both
			// since the stage is effectively instantaneous
			if (!inferredStart && inferredEnd) {
				inferredStart = inferredEnd;
			}

			// Update the previous stage's end if it's missing (this completes the chain)
			if (i > 0) {
				const prevStage = stageOrder[i - 1];
				const prevTiming = stageTiming.get(prevStage);
				if (
					prevTiming &&
					prevTiming.start &&
					!prevTiming.end &&
					inferredStart
				) {
					prevTiming.end = inferredStart;
					if (prevTiming.status === "running") {
						prevTiming.status = "completed";
					}
				}
			}

			// Set inferred timing - mark as completed since job completed successfully
			stageTiming.set(stage, {
				start: inferredStart,
				end: inferredEnd,
				status: "completed",
			});
		}
	}

	// Build the result
	return stageOrder.map((stage): StageDuration => {
		const timing = stageTiming.get(stage);

		if (!timing) {
			return {
				stage,
				durationSecs: 0,
				startedAt: null,
				completedAt: null,
				status: "pending",
			};
		}

		const durationSecs =
			timing.start && timing.end
				? calculateDurationSecs(timing.start, timing.end)
				: 0;

		let status: StageDuration["status"];
		if (timing.status === "completed") {
			status = "completed";
		} else if (timing.status === "failed") {
			// For failed jobs, mark the failed stage
			status = "completed"; // Still show it as processed
		} else if (timing.status === "running") {
			status = "running";
		} else {
			status = "pending";
		}

		return {
			stage,
			durationSecs,
			startedAt: timing.start,
			completedAt: timing.end,
			status,
		};
	});
}

/**
 * Calculate complete job metrics including performance comparison
 */
export function calculateJobMetrics(
	statusHistory: StatusTransition[],
	jobKickedAt: string,
	jobStatus: JobStatus,
	totalDurationSecs: number | null,
	avgDurationSecs: number | null,
): JobMetrics {
	const stageDurations = calculateStageDurations(
		statusHistory,
		jobKickedAt,
		jobStatus,
	);

	const actualTotal =
		totalDurationSecs ??
		stageDurations.reduce((sum, s) => sum + s.durationSecs, 0);

	let performanceRatio: number | null = null;
	if (avgDurationSecs && avgDurationSecs > 0 && actualTotal > 0) {
		performanceRatio = actualTotal / avgDurationSecs;
	}

	return {
		totalDurationSecs: actualTotal,
		stageDurations,
		avgDurationSecs,
		performanceRatio,
	};
}

/**
 * Format duration as human-readable string with units
 */
export function formatDurationWithUnits(seconds: number): string {
	if (seconds === 0) return "-";
	if (seconds < 60) return `${seconds}s`;
	if (seconds < 3600) {
		const mins = Math.floor(seconds / 60);
		const secs = seconds % 60;
		return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`;
	}
	const hours = Math.floor(seconds / 3600);
	const mins = Math.floor((seconds % 3600) / 60);
	return mins > 0 ? `${hours}h ${mins}m` : `${hours}h`;
}

/**
 * Get performance label based on ratio
 */
export function getPerformanceLabel(ratio: number | null): {
	label: string;
	color: "green" | "amber" | "red" | "gray";
} {
	if (ratio === null) {
		return { label: "-", color: "gray" };
	}

	if (ratio <= 0.8) {
		return { label: "Fast", color: "green" };
	} else if (ratio <= 1.2) {
		return { label: "Normal", color: "amber" };
	} else {
		return { label: "Slow", color: "red" };
	}
}

/**
 * Calculate the percentage width for a stage duration bar
 * relative to the maximum duration among all stages
 */
export function calculateBarWidth(
	durationSecs: number,
	maxDurationSecs: number,
): number {
	if (maxDurationSecs === 0 || durationSecs === 0) return 0;
	return Math.round((durationSecs / maxDurationSecs) * 100);
}

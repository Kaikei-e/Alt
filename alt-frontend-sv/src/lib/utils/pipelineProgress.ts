import {
	PIPELINE_STAGES,
	type PipelineStage,
	type SubStageProgress,
} from "$lib/schema/dashboard";

/**
 * Determines if a pipeline stage should show sub-stage progress (n/m format).
 * Currently supports evidence and dispatch stages.
 */
export function shouldShowSubStageProgress(
	stage: PipelineStage,
	status: "completed" | "running" | "pending",
	subStageProgress: SubStageProgress | null | undefined,
): boolean {
	if (status !== "running" || !subStageProgress) {
		return false;
	}

	// Evidence stage shows progress during evidence_building phase
	if (stage === "evidence" && subStageProgress.phase === "evidence_building") {
		return true;
	}

	// Dispatch stage shows progress during clustering or summarization phases
	if (
		stage === "dispatch" &&
		(subStageProgress.phase === "clustering" ||
			subStageProgress.phase === "summarization")
	) {
		return true;
	}

	return false;
}

/**
 * Formats the sub-stage progress as "n/m" string.
 */
export function formatSubStageProgress(
	subStageProgress: SubStageProgress,
): string {
	return `${subStageProgress.completed_genres}/${subStageProgress.total_genres}`;
}

/**
 * Infers whether a stage should be considered completed based on the position
 * of later stages that are running or completed.
 *
 * Logic: If any stage AFTER this one is running or completed,
 * this stage must have completed already (pipeline runs sequentially).
 *
 * This fixes the Evidence stage display bug where Evidence is not logged
 * as completed in the backend but Dispatch is already running.
 */
export function inferStageCompletion(
	stage: PipelineStage,
	stageIndex: number,
	stagesCompleted: string[],
	currentStage: string | null,
	currentStageIndex: number,
): "completed" | "running" | "pending" {
	// Already explicitly completed
	if (stagesCompleted.includes(stage)) {
		return "completed";
	}

	// Currently running
	if (currentStage === stage || stageIndex === currentStageIndex) {
		return "running";
	}

	// Infer completion: if current stage index > this stage's index,
	// this stage must be completed (pipeline runs sequentially)
	if (currentStageIndex > stageIndex) {
		return "completed";
	}

	// Also check if any later stage is in stagesCompleted
	const laterStageCompleted = PIPELINE_STAGES.slice(stageIndex + 1).some(
		(laterStage) => stagesCompleted.includes(laterStage),
	);

	if (laterStageCompleted) {
		return "completed";
	}

	return "pending";
}

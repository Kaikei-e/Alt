// Re-export types from domain layer
export type {
	SystemMetric,
	RecentActivity,
	LogError,
	AdminJob,
	TimeWindow,
	RecapJob,
	JobStatus,
	TriggerSource,
	GenreStatusType,
	StatusTransitionActor,
	PipelineStage,
	GenreProgressInfo,
	SubStagePhase,
	SubStageProgress,
	ActiveJobInfo,
	StatusTransition,
	RecentJobSummary,
	JobStats,
	UserJobContext,
	JobProgressEvent,
} from "$lib/domain/dashboard/types";

export { TIME_WINDOWS, PIPELINE_STAGES } from "$lib/domain/dashboard/types";

// Re-export format helpers from domain layer
export {
	getStageLabel,
	getStatusColor,
	getStatusBgColor,
	formatDuration,
} from "$lib/domain/dashboard/format";

// Re-export types from domain layer

// Re-export format helpers from domain layer
export {
	formatDuration,
	getStageLabel,
	getStatusBgColor,
	getStatusColor,
} from "$lib/domain/dashboard/format";
export type {
	ActiveJobInfo,
	AdminJob,
	GenreProgressInfo,
	GenreStatusType,
	JobProgressEvent,
	JobStats,
	JobStatus,
	LogError,
	PipelineStage,
	RecapJob,
	RecentActivity,
	RecentJobSummary,
	StatusTransition,
	StatusTransitionActor,
	SubStagePhase,
	SubStageProgress,
	SystemMetric,
	TimeWindow,
	TriggerSource,
	UserJobContext,
} from "$lib/domain/dashboard/types";
export { PIPELINE_STAGES, TIME_WINDOWS } from "$lib/domain/dashboard/types";

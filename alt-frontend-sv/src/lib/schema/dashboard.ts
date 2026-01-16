export interface SystemMetric {
	job_id: string | null;
	timestamp: string; // ISO 8601 format
	metrics: Record<string, unknown>;
}

export interface RecentActivity {
	job_id: string | null;
	metric_type: string;
	timestamp: string; // ISO 8601 format
}

export interface LogError {
	timestamp: string; // ISO 8601 format
	error_type: string;
	error_message: string | null;
	raw_line: string | null;
	service: string | null;
}

export interface AdminJob {
	job_id: string;
	kind: string;
	status: string;
	started_at: string; // ISO 8601 format
	finished_at: string | null; // ISO 8601 format
	payload: Record<string, unknown> | null;
	result: Record<string, unknown> | null;
	error: string | null;
}

export type TimeWindow = "4h" | "24h" | "3d" | "7d";

export const TIME_WINDOWS: Record<TimeWindow, number> = {
	"4h": 4 * 3600,
	"24h": 24 * 3600,
	"3d": 72 * 3600,
	"7d": 7 * 24 * 3600,
};

export interface RecapJob {
	job_id: string;
	status: string;
	last_stage: string | null;
	kicked_at: string;
	updated_at: string;
}

// ============================================================================
// Job Progress Dashboard Types
// ============================================================================

export type JobStatus = "pending" | "running" | "completed" | "failed";
export type TriggerSource = "system" | "user";
export type GenreStatusType = "pending" | "running" | "succeeded" | "failed";
export type StatusTransitionActor = "system" | "scheduler" | "manual_repair" | "migration_backfill";

export const PIPELINE_STAGES = [
	"fetch",
	"preprocess",
	"dedup",
	"genre",
	"select",
	"evidence",
	"dispatch",
	"persist",
] as const;

export type PipelineStage = (typeof PIPELINE_STAGES)[number];

export interface GenreProgressInfo {
	status: GenreStatusType;
	cluster_count: number | null;
	article_count: number | null;
}

export interface ActiveJobInfo {
	job_id: string;
	status: JobStatus;
	current_stage: string | null;
	stage_index: number;
	stages_completed: string[];
	genre_progress: Record<string, GenreProgressInfo>;
	total_articles: number | null;
	user_article_count: number | null;
	kicked_at: string;
	trigger_source: TriggerSource;
}

export interface StatusTransition {
	id: number;
	status: JobStatus;
	stage: string | null;
	transitioned_at: string;
	reason: string | null;
	actor: StatusTransitionActor;
}

export interface RecentJobSummary {
	job_id: string;
	status: JobStatus;
	last_stage: string | null;
	kicked_at: string;
	updated_at: string;
	duration_secs: number | null;
	trigger_source: TriggerSource;
	user_id: string | null;
	status_history: StatusTransition[];
}

export interface JobStats {
	success_rate_24h: number;
	avg_duration_secs: number | null;
	total_jobs_24h: number;
	running_jobs: number;
	failed_jobs_24h: number;
}

export interface UserJobContext {
	user_article_count: number;
	user_jobs_count: number;
	user_feed_ids: string[];
}

export interface JobProgressEvent {
	active_job: ActiveJobInfo | null;
	recent_jobs: RecentJobSummary[];
	stats: JobStats;
	user_context: UserJobContext | null;
}

export function getStageLabel(stage: PipelineStage): string {
	const labels: Record<PipelineStage, string> = {
		fetch: "Fetch",
		preprocess: "Preprocess",
		dedup: "Dedup",
		genre: "Genre",
		select: "Select",
		evidence: "Evidence",
		dispatch: "Dispatch",
		persist: "Persist",
	};
	return labels[stage];
}

export function getStatusColor(status: JobStatus | GenreStatusType): string {
	const colors: Record<JobStatus | GenreStatusType, string> = {
		pending: "text-gray-500",
		running: "text-blue-500",
		completed: "text-green-500",
		succeeded: "text-green-500",
		failed: "text-red-500",
	};
	return colors[status] ?? "text-gray-500";
}

export function getStatusBgColor(status: JobStatus | GenreStatusType): string {
	const colors: Record<JobStatus | GenreStatusType, string> = {
		pending: "bg-gray-100",
		running: "bg-blue-100",
		completed: "bg-green-100",
		succeeded: "bg-green-100",
		failed: "bg-red-100",
	};
	return colors[status] ?? "bg-gray-100";
}

export function formatDuration(seconds: number | null): string {
	if (seconds === null) return "-";
	if (seconds < 60) return `${seconds}s`;
	if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
	const hours = Math.floor(seconds / 3600);
	const minutes = Math.floor((seconds % 3600) / 60);
	return `${hours}h ${minutes}m`;
}

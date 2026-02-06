export interface SystemMetric {
	job_id: string | null;
	timestamp: string;
	metrics: Record<string, unknown>;
}

export interface RecentActivity {
	job_id: string | null;
	metric_type: string;
	timestamp: string;
}

export interface LogError {
	timestamp: string;
	error_type: string;
	error_message: string | null;
	raw_line: string | null;
	service: string | null;
}

export interface AdminJob {
	job_id: string;
	kind: string;
	status: string;
	started_at: string;
	finished_at: string | null;
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

export type JobStatus = "pending" | "running" | "completed" | "failed";
export type TriggerSource = "system" | "user";
export type GenreStatusType = "pending" | "running" | "succeeded" | "failed";
export type StatusTransitionActor =
	| "system"
	| "scheduler"
	| "manual_repair"
	| "migration_backfill";

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

export type SubStagePhase =
	| "evidence_building"
	| "clustering"
	| "summarization";

export interface SubStageProgress {
	phase: SubStagePhase;
	total_genres: number;
	completed_genres: number;
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
	sub_stage_progress: SubStageProgress | null;
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

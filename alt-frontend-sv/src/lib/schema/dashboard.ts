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

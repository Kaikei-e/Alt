import type {
	AdminJob,
	JobProgressEvent,
	JobStats,
	LogError,
	RecapJob,
	RecentActivity,
	SystemMetric,
} from "$lib/schema/dashboard";
import { callClientAPI } from "./core";

export async function getMetrics(
	metricType?: string,
	windowSeconds?: number,
	limit?: number,
): Promise<SystemMetric[]> {
	const params = new URLSearchParams();
	if (metricType) {
		params.set("type", metricType);
	}
	if (windowSeconds !== undefined) {
		params.set("window", windowSeconds.toString());
	}
	if (limit !== undefined) {
		params.set("limit", limit.toString());
	}

	const queryString = params.toString();
	const endpoint = queryString
		? `/v1/dashboard/metrics?${queryString}`
		: "/v1/dashboard/metrics";

	return callClientAPI<SystemMetric[]>(endpoint);
}

export async function getOverview(
	windowSeconds?: number,
	limit?: number,
): Promise<RecentActivity[]> {
	const params = new URLSearchParams();
	if (windowSeconds !== undefined) {
		params.set("window", windowSeconds.toString());
	}
	if (limit !== undefined) {
		params.set("limit", limit.toString());
	}

	const queryString = params.toString();
	const endpoint = queryString
		? `/v1/dashboard/overview?${queryString}`
		: "/v1/dashboard/overview";

	return callClientAPI<RecentActivity[]>(endpoint);
}

export async function getLogs(
	windowSeconds?: number,
	limit?: number,
): Promise<LogError[]> {
	const params = new URLSearchParams();
	if (windowSeconds !== undefined) {
		params.set("window", windowSeconds.toString());
	}
	if (limit !== undefined) {
		params.set("limit", limit.toString());
	}

	const queryString = params.toString();
	const endpoint = queryString
		? `/v1/dashboard/logs?${queryString}`
		: "/v1/dashboard/logs";

	return callClientAPI<LogError[]>(endpoint);
}

export async function getJobs(
	windowSeconds?: number,
	limit?: number,
): Promise<AdminJob[]> {
	const params = new URLSearchParams();
	if (windowSeconds !== undefined) {
		params.set("window", windowSeconds.toString());
	}
	if (limit !== undefined) {
		params.set("limit", limit.toString());
	}

	const queryString = params.toString();
	const endpoint = queryString
		? `/v1/dashboard/jobs?${queryString}`
		: "/v1/dashboard/jobs";

	return callClientAPI<AdminJob[]>(endpoint);
}

import { base } from "$app/paths";

export async function getRecapJobs(
	fetch: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>,
	windowSeconds?: number,
	limit?: number,
): Promise<RecapJob[]> {
	const params = new URLSearchParams();
	if (windowSeconds) params.set("window", windowSeconds.toString());
	if (limit) params.set("limit", limit.toString());

	const res = await fetch(
		`${base}/api/v1/dashboard/recap_jobs?${params.toString()}`,
	);
	if (!res.ok) {
		throw new Error("Failed to fetch recap jobs");
	}
	return res.json();
}

// ============================================================================
// Job Progress Dashboard API
// ============================================================================

export async function getJobProgress(
	fetch: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>,
	options?: {
		userId?: string;
		windowSeconds?: number;
		limit?: number;
	},
): Promise<JobProgressEvent> {
	const params = new URLSearchParams();
	if (options?.userId) params.set("user_id", options.userId);
	if (options?.windowSeconds)
		params.set("window", options.windowSeconds.toString());
	if (options?.limit) params.set("limit", options.limit.toString());

	const queryString = params.toString();
	const url = queryString
		? `${base}/api/v1/dashboard/job-progress?${queryString}`
		: `${base}/api/v1/dashboard/job-progress`;

	const res = await fetch(url);
	if (!res.ok) {
		throw new Error("Failed to fetch job progress");
	}
	return res.json();
}

export async function getJobStats(
	fetch: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>,
): Promise<JobStats> {
	const res = await fetch(`${base}/api/v1/dashboard/job-stats`);
	if (!res.ok) {
		throw new Error("Failed to fetch job stats");
	}
	return res.json();
}

// ============================================================================
// Job Trigger API
// ============================================================================

export interface TriggerJobResponse {
	job_id: string;
	genres: string[];
	status: string;
}

export async function triggerRecapJob(
	fetch: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>,
	genres?: string[],
): Promise<TriggerJobResponse> {
	const res = await fetch(`${base}/api/v1/generate/recaps/7days`, {
		method: "POST",
		headers: {
			"Content-Type": "application/json",
		},
		body: JSON.stringify(genres ? { genres } : {}),
	});
	if (!res.ok) {
		const error = await res.text();
		throw new Error(`Failed to trigger job: ${error}`);
	}
	return res.json();
}

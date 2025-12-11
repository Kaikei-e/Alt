import { callClientAPI } from "./core";
import type {
  AdminJob,
  LogError,
  RecentActivity,
  SystemMetric,
} from "$lib/schema/dashboard";

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


export async function getRecapJobs(
  fetch: (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>,
  windowSeconds?: number,
  limit?: number
): Promise<RecapJob[]> {
  const params = new URLSearchParams();
  if (windowSeconds) params.set('window', windowSeconds.toString());
  if (limit) params.set('limit', limit.toString());

  const res = await fetch(`/api/v1/dashboard/recap_jobs?${params.toString()}`);
  if (!res.ok) {
    throw new Error('Failed to fetch recap jobs');
  }
  return res.json();
}

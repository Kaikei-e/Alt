import type { RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { proxyDashboardGet } from "$lib/server/dashboard-proxy";

const RECAP_WORKER_URL =
	env.RECAP_WORKER_BASE_URL || "http://recap-worker:9005";

export const GET: RequestHandler = (event) =>
	proxyDashboardGet(RECAP_WORKER_URL, "/v1/dashboard/job-stats", event, {
		errorLabel: "Recap Worker API error",
	});

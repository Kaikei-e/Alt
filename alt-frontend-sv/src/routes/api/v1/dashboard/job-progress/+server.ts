import type { RequestHandler } from "@sveltejs/kit";
import { proxyDashboardGet } from "#lib/server/dashboard-proxy.js";
import { getUserRole } from "#lib/server/user-role.js";
import { RECAP_WORKER_BASE_URL } from "$app/env/private";

const RECAP_WORKER_URL = RECAP_WORKER_BASE_URL || "http://recap-worker:9005";

export const GET: RequestHandler = (event) => {
	const isAdmin = getUserRole(event.locals?.user ?? null) === "admin";
	return proxyDashboardGet(
		RECAP_WORKER_URL,
		"/v1/dashboard/job-progress",
		event,
		{
			allowedParams: isAdmin
				? ["user_id", "window", "limit"]
				: ["window", "limit"],
			errorLabel: "Recap Worker API error",
		},
	);
};

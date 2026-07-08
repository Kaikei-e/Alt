import type { RequestHandler } from "@sveltejs/kit";
import { env } from "$env/dynamic/private";
import { proxyDashboardGet } from "$lib/server/dashboard-proxy";

const BACKEND_URL =
	env.BACKEND_CONNECT_URL || "http://alt-butterfly-facade:9250";

export const GET: RequestHandler = (event) =>
	proxyDashboardGet(BACKEND_URL, "/v1/dashboard/metrics", event, {
		allowedParams: ["type", "window", "limit"],
	});

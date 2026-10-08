import type { RequestHandler } from "@sveltejs/kit";
import { proxyDashboardGet } from "#lib/server/dashboard-proxy.js";
import { BACKEND_CONNECT_URL } from "$app/env/private";

const BACKEND_URL = BACKEND_CONNECT_URL || "http://alt-butterfly-facade:9250";

export const GET: RequestHandler = (event) =>
	proxyDashboardGet(BACKEND_URL, "/v1/dashboard/jobs", event, {
		allowedParams: ["window", "limit"],
	});

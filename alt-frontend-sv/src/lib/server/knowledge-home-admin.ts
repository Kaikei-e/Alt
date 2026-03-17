import { createConnectTransport } from "@connectrpc/connect-web";
import { env } from "$env/dynamic/private";
import type {
	FeatureFlagsConfigData,
	ProjectionHealthData,
} from "$lib/connect/knowledge_home_admin";
import {
	getFeatureFlags,
	getProjectionHealth,
	pauseBackfill,
	resumeBackfill,
	triggerBackfill,
} from "$lib/connect/knowledge_home_admin";

const BFF_CONNECT_URL =
	env.BFF_CONNECT_URL || "http://alt-butterfly-facade:9250";

export interface KnowledgeHomeAdminSnapshot {
	health: ProjectionHealthData | null;
	flags: FeatureFlagsConfigData | null;
}

function createBffTransport(backendToken: string) {
	return createConnectTransport({
		baseUrl: BFF_CONNECT_URL,
		interceptors: [
			(next) => async (req) => {
				req.header.set("X-Alt-Backend-Token", backendToken);
				return next(req);
			},
		],
	});
}

export async function fetchKnowledgeHomeAdminSnapshot(
	backendToken: string,
): Promise<KnowledgeHomeAdminSnapshot> {
	const transport = createBffTransport(backendToken);
	const [health, flags] = await Promise.all([
		getProjectionHealth(transport),
		getFeatureFlags(transport),
	]);

	return {
		health,
		flags,
	};
}

export async function triggerKnowledgeHomeBackfill(
	backendToken: string,
	projectionVersion: number,
) {
	const transport = createBffTransport(backendToken);
	return triggerBackfill(transport, projectionVersion);
}

export async function pauseKnowledgeHomeBackfill(
	backendToken: string,
	jobId: string,
) {
	const transport = createBffTransport(backendToken);
	return pauseBackfill(transport, jobId);
}

export async function resumeKnowledgeHomeBackfill(
	backendToken: string,
	jobId: string,
) {
	const transport = createBffTransport(backendToken);
	return resumeBackfill(transport, jobId);
}

import { createConnectTransport } from "@connectrpc/connect-web";
import { env } from "$env/dynamic/private";
import type {
	FeatureFlagsConfigData,
	ProjectionHealthData,
} from "$lib/connect/knowledge_home_admin";
import {
	getFeatureFlags,
	getProjectionHealth,
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

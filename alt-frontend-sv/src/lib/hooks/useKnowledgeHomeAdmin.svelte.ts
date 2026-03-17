/**
 * Headless composable for Knowledge Home Admin data fetching.
 * Polls every 10s for health status.
 */
import { createClientTransport } from "$lib/connect";
import {
	getProjectionHealth,
	getFeatureFlags,
	type ProjectionHealthData,
	type FeatureFlagsConfigData,
} from "$lib/connect/knowledge_home_admin";

export function useKnowledgeHomeAdmin() {
	let health = $state<ProjectionHealthData | null>(null);
	let flags = $state<FeatureFlagsConfigData | null>(null);
	let loading = $state(false);
	let error = $state<Error | null>(null);

	let pollTimer: ReturnType<typeof setInterval> | null = null;

	const fetchData = async () => {
		try {
			loading = true;
			error = null;
			const transport = createClientTransport();
			const [healthData, flagsData] = await Promise.all([
				getProjectionHealth(transport),
				getFeatureFlags(transport),
			]);
			health = healthData;
			flags = flagsData;
		} catch (err) {
			error = err instanceof Error ? err : new Error("Unknown error");
		} finally {
			loading = false;
		}
	};

	const startPolling = (intervalMs = 10000) => {
		stopPolling();
		fetchData();
		pollTimer = setInterval(fetchData, intervalMs);
	};

	const stopPolling = () => {
		if (pollTimer) {
			clearInterval(pollTimer);
			pollTimer = null;
		}
	};

	return {
		get health() {
			return health;
		},
		get flags() {
			return flags;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		fetchData,
		startPolling,
		stopPolling,
	};
}

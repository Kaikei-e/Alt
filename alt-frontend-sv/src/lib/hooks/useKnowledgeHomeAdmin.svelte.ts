import type {
	FeatureFlagsConfigData,
	ProjectionHealthData,
} from "$lib/connect/knowledge_home_admin";

interface Snapshot {
	health: ProjectionHealthData | null;
	flags: FeatureFlagsConfigData | null;
}

export function useKnowledgeHomeAdmin(fetcher: () => Promise<Snapshot>) {
	let health = $state<ProjectionHealthData | null>(null);
	let flags = $state<FeatureFlagsConfigData | null>(null);
	let error = $state<Error | null>(null);
	let refreshing = $state(false);
	let lastUpdatedAt = $state<Date | null>(null);

	let pollTimer: ReturnType<typeof setInterval> | null = null;
	let inFlight: Promise<void> | null = null;

	const fetchData = async () => {
		if (inFlight) {
			return inFlight;
		}

		inFlight = (async () => {
		try {
			refreshing = true;
			const snapshot = await fetcher();
			health = snapshot.health;
			flags = snapshot.flags;
			error = null;
			lastUpdatedAt = new Date();
		} catch (err) {
			error = err instanceof Error ? err : new Error("Unknown error");
		} finally {
			refreshing = false;
			inFlight = null;
		}
		})();

		return inFlight;
	};

	const startPolling = (intervalMs = 10000) => {
		stopPolling();
		void fetchData();
		pollTimer = setInterval(fetchData, intervalMs);
	};

	const stopPolling = () => {
		if (pollTimer) {
			clearInterval(pollTimer);
			pollTimer = null;
		}
	};

	const seed = (snapshot: Snapshot, seedError: Error | null = null) => {
		health = snapshot.health;
		flags = snapshot.flags;
		error = seedError;
		lastUpdatedAt = new Date();
	};

	return {
		seed,
		get health() {
			return health;
		},
		get flags() {
			return flags;
		},
		get error() {
			return error;
		},
		get refreshing() {
			return refreshing;
		},
		get lastUpdatedLabel() {
			if (!lastUpdatedAt) return "never";
			return lastUpdatedAt.toLocaleTimeString("ja-JP");
		},
		fetchData,
		startPolling,
		stopPolling,
	};
}

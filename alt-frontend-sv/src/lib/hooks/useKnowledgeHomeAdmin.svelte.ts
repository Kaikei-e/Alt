import type {
	FeatureFlagsConfigData,
	ProjectionHealthData,
} from "$lib/connect/knowledge_home_admin";

interface Snapshot {
	health: ProjectionHealthData | null;
	flags: FeatureFlagsConfigData | null;
}

export type KnowledgeHomeAdminSnapshot = Snapshot;
export type KnowledgeHomeAdminActionRequest =
	| {
			action: "trigger";
			projectionVersion: number;
	  }
	| {
			action: "pause" | "resume";
			jobId: string;
	  };

export function useKnowledgeHomeAdmin(
	fetcher: () => Promise<Snapshot>,
	actionRunner?: (action: KnowledgeHomeAdminActionRequest) => Promise<void>,
) {
	let health = $state<ProjectionHealthData | null>(null);
	let flags = $state<FeatureFlagsConfigData | null>(null);
	let error = $state<Error | null>(null);
	let refreshing = $state(false);
	let lastUpdatedAt = $state<Date | null>(null);
	let acting = $state(false);
	let activeJobId = $state<string | null>(null);

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

	const runAction = async (action: KnowledgeHomeAdminActionRequest) => {
		if (!actionRunner) {
			throw new Error("Admin actions are unavailable.");
		}

		acting = true;
		activeJobId = "jobId" in action ? action.jobId : null;

		try {
			await actionRunner(action);
			await fetchData();
		} catch (err) {
			error = err instanceof Error ? err : new Error("Unknown error");
		} finally {
			acting = false;
			activeJobId = null;
		}
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
		get acting() {
			return acting;
		},
		get activeJobId() {
			return activeJobId;
		},
		get lastUpdatedLabel() {
			if (!lastUpdatedAt) return "never";
			return lastUpdatedAt.toLocaleTimeString("ja-JP");
		},
		triggerBackfill: async (projectionVersion: number) =>
			runAction({ action: "trigger", projectionVersion }),
		pauseBackfill: async (jobId: string) =>
			runAction({ action: "pause", jobId }),
		resumeBackfill: async (jobId: string) =>
			runAction({ action: "resume", jobId }),
		fetchData,
		startPolling,
		stopPolling,
	};
}

import type {
	FeatureFlagsConfigData,
	ProjectionHealthData,
	SLOStatusData,
	ReprojectRunData,
	ReprojectDiffSummaryData,
	ProjectionAuditData,
} from "$lib/connect/knowledge_home_admin";

interface Snapshot {
	health: ProjectionHealthData | null;
	flags: FeatureFlagsConfigData | null;
	sloStatus: SLOStatusData | null;
	reprojectRuns: ReprojectRunData[];
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
	  }
	| {
			action: "start_reproject";
			mode: string;
			fromVersion: string;
			toVersion: string;
			rangeStart?: string;
			rangeEnd?: string;
	  }
	| {
			action: "compare_reproject";
			reprojectRunId: string;
	  }
	| {
			action: "swap_reproject";
			reprojectRunId: string;
	  }
	| {
			action: "rollback_reproject";
			reprojectRunId: string;
	  }
	| {
			action: "run_audit";
			projectionName: string;
			projectionVersion: string;
			sampleSize: number;
	  };

export function useKnowledgeHomeAdmin(
	fetcher: () => Promise<Snapshot>,
	actionRunner?: (action: KnowledgeHomeAdminActionRequest) => Promise<void>,
) {
	let health = $state<ProjectionHealthData | null>(null);
	let flags = $state<FeatureFlagsConfigData | null>(null);
	let sloStatus = $state<SLOStatusData | null>(null);
	let reprojectRuns = $state<ReprojectRunData[]>([]);
	let reprojectDiff = $state<ReprojectDiffSummaryData | null>(null);
	let auditResult = $state<ProjectionAuditData | null>(null);
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
				sloStatus = snapshot.sloStatus;
				reprojectRuns = snapshot.reprojectRuns;
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
		sloStatus = snapshot.sloStatus;
		reprojectRuns = snapshot.reprojectRuns;
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

	const startReprojectAction = async (
		mode: string,
		fromVersion: string,
		toVersion: string,
		rangeStart?: string,
		rangeEnd?: string,
	) =>
		runAction({
			action: "start_reproject",
			mode,
			fromVersion,
			toVersion,
			rangeStart,
			rangeEnd,
		});

	const compareReprojectAction = async (reprojectRunId: string) => {
		if (!actionRunner) {
			throw new Error("Admin actions are unavailable.");
		}

		acting = true;
		try {
			const response = await fetch("/api/admin/knowledge-home", {
				method: "POST",
				credentials: "include",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ action: "compare_reproject", reprojectRunId }),
			});
			if (!response.ok) {
				const body = await response.json().catch(() => null);
				throw new Error(body?.error ?? "Failed to compare reproject.");
			}
			const result = await response.json();
			reprojectDiff = result.diff ?? null;
		} catch (err) {
			error = err instanceof Error ? err : new Error("Unknown error");
		} finally {
			acting = false;
		}
	};

	const swapReprojectAction = async (reprojectRunId: string) =>
		runAction({ action: "swap_reproject", reprojectRunId });

	const rollbackReprojectAction = async (reprojectRunId: string) =>
		runAction({ action: "rollback_reproject", reprojectRunId });

	const runAuditAction = async (
		projectionName: string,
		projectionVersion: string,
		sampleSize: number,
	) => {
		if (!actionRunner) throw new Error("Admin actions are unavailable.");
		acting = true;
		try {
			const response = await fetch("/api/admin/knowledge-home", {
				method: "POST",
				credentials: "include",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({
					action: "run_audit",
					projectionName,
					projectionVersion,
					sampleSize,
				}),
			});
			if (!response.ok) {
				const body = await response.json().catch(() => null);
				throw new Error(body?.error ?? "Failed to run audit.");
			}
			const result = await response.json();
			auditResult = result.audit ?? null;
		} catch (err) {
			error = err instanceof Error ? err : new Error("Unknown error");
		} finally {
			acting = false;
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
		get sloStatus() {
			return sloStatus;
		},
		get reprojectRuns() {
			return reprojectRuns;
		},
		get reprojectDiff() {
			return reprojectDiff;
		},
		get auditResult() {
			return auditResult;
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
		startReproject: startReprojectAction,
		compareReproject: compareReprojectAction,
		swapReproject: swapReprojectAction,
		rollbackReproject: rollbackReprojectAction,
		runAudit: runAuditAction,
		fetchData,
		startPolling,
		stopPolling,
	};
}

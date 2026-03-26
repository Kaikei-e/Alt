import type {
	TableStorageInfo,
	SnapshotMetadata,
	RetentionLogEntry,
	EligiblePartitionsResult,
	RetentionRunResponse,
	SovereignAdminSnapshot,
} from "$lib/types/sovereign-admin";

export type SovereignAdminActionRequest =
	| { action: "create_snapshot" }
	| { action: "run_retention"; dry_run: boolean };

export function useSovereignAdmin(
	fetcher: () => Promise<SovereignAdminSnapshot>,
	actionRunner?: (action: SovereignAdminActionRequest) => Promise<unknown>,
) {
	let storageStats = $state.raw<TableStorageInfo[]>([]);
	let snapshots = $state.raw<SnapshotMetadata[]>([]);
	let latestSnapshot = $state.raw<SnapshotMetadata | null>(null);
	let retentionLogs = $state.raw<RetentionLogEntry[]>([]);
	let eligiblePartitions = $state.raw<EligiblePartitionsResult[]>([]);
	let retentionResult = $state<RetentionRunResponse | null>(null);
	let error = $state<Error | null>(null);
	let refreshing = $state(false);
	let acting = $state(false);

	let pollTimer: ReturnType<typeof setInterval> | null = null;
	let inFlight: Promise<void> | null = null;

	const fetchData = async () => {
		if (inFlight) return inFlight;

		inFlight = (async () => {
			try {
				refreshing = true;
				const snapshot = await fetcher();
				storageStats = snapshot.storageStats;
				snapshots = snapshot.snapshots;
				latestSnapshot = snapshot.latestSnapshot;
				retentionLogs = snapshot.retentionLogs;
				eligiblePartitions = snapshot.eligiblePartitions;
				error = null;
			} catch (err) {
				error = err instanceof Error ? err : new Error("Unknown error");
			} finally {
				refreshing = false;
				inFlight = null;
			}
		})();

		return inFlight;
	};

	const startPolling = (intervalMs = 30000) => {
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

	const seed = (
		snapshot: SovereignAdminSnapshot,
		seedError: Error | null = null,
	) => {
		storageStats = snapshot.storageStats;
		snapshots = snapshot.snapshots;
		latestSnapshot = snapshot.latestSnapshot;
		retentionLogs = snapshot.retentionLogs;
		eligiblePartitions = snapshot.eligiblePartitions;
		error = seedError;
	};

	const createSnapshot = async () => {
		if (!actionRunner) throw new Error("Actions unavailable.");
		acting = true;
		try {
			await actionRunner({ action: "create_snapshot" });
			await fetchData();
		} catch (err) {
			error = err instanceof Error ? err : new Error("Unknown error");
		} finally {
			acting = false;
		}
	};

	const runRetention = async (dryRun: boolean) => {
		if (!actionRunner) throw new Error("Actions unavailable.");
		acting = true;
		try {
			const response = await fetch("/api/admin/knowledge-home/sovereign", {
				method: "POST",
				credentials: "include",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ action: "run_retention", dry_run: dryRun }),
			});
			if (!response.ok) {
				const body = await response.json().catch(() => null);
				throw new Error(body?.error ?? "Failed to run retention.");
			}
			const result = await response.json();
			retentionResult = result.result ?? null;
			await fetchData();
		} catch (err) {
			error = err instanceof Error ? err : new Error("Unknown error");
		} finally {
			acting = false;
		}
	};

	return {
		seed,
		get storageStats() {
			return storageStats;
		},
		get snapshots() {
			return snapshots;
		},
		get latestSnapshot() {
			return latestSnapshot;
		},
		get retentionLogs() {
			return retentionLogs;
		},
		get eligiblePartitions() {
			return eligiblePartitions;
		},
		get retentionResult() {
			return retentionResult;
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
		createSnapshot,
		runRetention,
		fetchData,
		startPolling,
		stopPolling,
	};
}

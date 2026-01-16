import { getJobProgress } from "$lib/api/client/dashboard";
import type { JobProgressEvent, TimeWindow } from "$lib/schema/dashboard";
import { TIME_WINDOWS } from "$lib/schema/dashboard";
import { onDestroy } from "svelte";

interface JobProgressState {
	data: JobProgressEvent | null;
	loading: boolean;
	error: string | null;
	isPolling: boolean;
}

interface UseJobProgressOptions {
	/** User ID for filtering user-specific jobs */
	userId?: string;
	/** Initial time window (default: 24h) */
	initialWindow?: TimeWindow;
	/** Auto-polling interval in ms (default: 5000ms, 0 to disable) */
	pollInterval?: number;
}

/**
 * Hook for fetching and managing job progress data.
 * Supports automatic polling for real-time updates.
 */
export function useJobProgress(options: UseJobProgressOptions = {}) {
	const { userId, initialWindow = "24h", pollInterval = 5000 } = options;

	let data = $state<JobProgressEvent | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let currentWindow = $state<TimeWindow>(initialWindow);
	let isPolling = $state(false);
	let pollTimeoutId: ReturnType<typeof setTimeout> | null = null;

	async function fetchData(window?: TimeWindow) {
		const targetWindow = window ?? currentWindow;
		loading = true;
		error = null;

		try {
			const response = await getJobProgress(fetch, {
				userId,
				windowSeconds: TIME_WINDOWS[targetWindow],
				limit: 50,
			});
			data = response;
			currentWindow = targetWindow;
		} catch (err) {
			error =
				err instanceof Error ? err.message : "Failed to fetch job progress";
			console.error("useJobProgress error:", err);
		} finally {
			loading = false;
		}
	}

	function setWindow(window: TimeWindow) {
		if (window !== currentWindow) {
			fetchData(window);
		}
	}

	function startPolling() {
		if (pollInterval <= 0 || isPolling) return;

		isPolling = true;

		const poll = async () => {
			if (!isPolling) return;

			try {
				const response = await getJobProgress(fetch, {
					userId,
					windowSeconds: TIME_WINDOWS[currentWindow],
					limit: 50,
				});
				data = response;
				error = null;
			} catch (err) {
				console.error("useJobProgress polling error:", err);
			}

			if (isPolling) {
				pollTimeoutId = setTimeout(poll, pollInterval);
			}
		};

		pollTimeoutId = setTimeout(poll, pollInterval);
	}

	function stopPolling() {
		isPolling = false;
		if (pollTimeoutId) {
			clearTimeout(pollTimeoutId);
			pollTimeoutId = null;
		}
	}

	function refresh() {
		return fetchData();
	}

	// Cleanup on destroy
	onDestroy(() => {
		stopPolling();
	});

	return {
		get data() {
			return data;
		},
		get loading() {
			return loading;
		},
		get error() {
			return error;
		},
		get currentWindow() {
			return currentWindow;
		},
		get isPolling() {
			return isPolling;
		},
		fetchData,
		setWindow,
		startPolling,
		stopPolling,
		refresh,
	};
}

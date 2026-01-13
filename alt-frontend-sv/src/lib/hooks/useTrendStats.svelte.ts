import { getTrendStats } from "$lib/api/client/stats";
import type { TrendDataResponse, TimeWindow } from "$lib/schema/stats";

interface TrendStatsState {
	data: TrendDataResponse | null;
	loading: boolean;
	error: string | null;
}

/**
 * Hook for fetching and managing trend statistics data.
 * Provides reactive state that updates when the time window changes.
 */
export function useTrendStats() {
	let data = $state<TrendDataResponse | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let currentWindow = $state<TimeWindow>("24h");

	async function fetchData(window: TimeWindow) {
		loading = true;
		error = null;

		try {
			const response = await getTrendStats(window);
			data = response;
			currentWindow = window;
		} catch (err) {
			error = err instanceof Error ? err.message : "Failed to fetch trend data";
			console.error("useTrendStats error:", err);
		} finally {
			loading = false;
		}
	}

	function setWindow(window: TimeWindow) {
		if (window !== currentWindow) {
			fetchData(window);
		}
	}

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
		fetchData,
		setWindow,
	};
}

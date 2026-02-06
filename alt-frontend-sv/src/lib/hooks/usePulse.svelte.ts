/**
 * Headless composable for Evening Pulse data fetching and state management.
 * Shared by both Desktop and Mobile evening-pulse pages.
 */
import { goto } from "$app/navigation";
import { ConnectError, Code } from "@connectrpc/connect";
import { createClientTransport, getEveningPulse } from "$lib/connect";
import type { EveningPulse, PulseTopic } from "$lib/schema/evening_pulse";

export function usePulse() {
	let data = $state<EveningPulse | null>(null);
	let isLoading = $state(true);
	let error = $state<Error | null>(null);
	let isRetrying = $state(false);
	let selectedTopic = $state<PulseTopic | null>(null);

	const fetchData = async () => {
		try {
			isLoading = true;
			error = null;
			const transport = createClientTransport();
			data = await getEveningPulse(transport);
		} catch (err) {
			if (err instanceof ConnectError) {
				if (err.code === Code.Unauthenticated) {
					goto("/login");
					return;
				}
				if (err.code === Code.NotFound) {
					data = null;
					error = null;
					return;
				}
			}
			error = err instanceof Error ? err : new Error("Unknown error");
			data = null;
		} finally {
			isLoading = false;
		}
	};

	const retry = async () => {
		isRetrying = true;
		try {
			await fetchData();
		} catch (err) {
			console.error("Retry failed:", err);
		} finally {
			isRetrying = false;
		}
	};

	const selectTopic = (clusterId: number) => {
		const topic = data?.topics.find((t) => t.clusterId === clusterId);
		if (topic) {
			selectedTopic = topic;
		}
		return topic ?? null;
	};

	const clearSelectedTopic = () => {
		selectedTopic = null;
	};

	return {
		get data() {
			return data;
		},
		get isLoading() {
			return isLoading;
		},
		get error() {
			return error;
		},
		get isRetrying() {
			return isRetrying;
		},
		get selectedTopic() {
			return selectedTopic;
		},
		fetchData,
		retry,
		selectTopic,
		clearSelectedTopic,
	};
}

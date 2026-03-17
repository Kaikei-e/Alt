/**
 * Hook for RecallRail state management.
 * Fetches recall candidates and provides snooze/dismiss actions.
 */
import { ConnectError, Code } from "@connectrpc/connect";
import {
	createClientTransport,
	getRecallRailCandidates,
	snoozeRecallItem,
	dismissRecallItem,
} from "$lib/connect";
import type { RecallCandidateData } from "$lib/connect/knowledge_home";

export function useRecallRail() {
	let candidates = $state<RecallCandidateData[]>([]);
	let loading = $state(false);
	let error = $state<Error | null>(null);

	const fetchCandidates = async (limit = 5) => {
		try {
			loading = true;
			error = null;
			const transport = createClientTransport();
			candidates = await getRecallRailCandidates(transport, limit);
		} catch (err) {
			if (err instanceof ConnectError && err.code === Code.PermissionDenied) {
				// Feature not enabled — not an error
				candidates = [];
				return;
			}
			error = err instanceof Error ? err : new Error("Unknown error");
		} finally {
			loading = false;
		}
	};

	const snooze = async (itemKey: string, hours = 24) => {
		// Optimistic removal
		candidates = candidates.filter((c) => c.itemKey !== itemKey);
		try {
			const transport = createClientTransport();
			await snoozeRecallItem(transport, itemKey, hours);
		} catch {
			// Fire-and-forget
		}
	};

	const dismiss = async (itemKey: string) => {
		// Optimistic removal
		candidates = candidates.filter((c) => c.itemKey !== itemKey);
		try {
			const transport = createClientTransport();
			await dismissRecallItem(transport, itemKey);
		} catch {
			// Fire-and-forget
		}
	};

	return {
		get candidates() { return candidates; },
		get loading() { return loading; },
		get error() { return error; },
		fetchCandidates,
		snooze,
		dismiss,
	};
}

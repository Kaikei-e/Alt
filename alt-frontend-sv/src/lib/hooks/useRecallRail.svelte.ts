/**
 * Hook for RecallRail state management.
 *
 * ADR-000913 §D-9 single-source migration: candidates flow only from the
 * GetKnowledgeHome payload (consumed via useKnowledgeHome). Snooze and
 * dismiss dispatch through the unified TrackHomeAction RPC so the legacy
 * GetRecallRail / TrackRecallAction endpoints can retire.
 */
import { createClientTransport, trackHomeAction } from "$lib/connect";
import type { RecallCandidateData } from "$lib/connect/knowledge_home";

export function useRecallRail() {
	let candidates = $state<RecallCandidateData[]>([]);

	const snooze = async (itemKey: string, hours = 24) => {
		// Optimistic removal: keep the rail responsive even if the backend
		// call lags. The server-side projection is idempotent — replays
		// of the same snooze are a no-op.
		candidates = candidates.filter((c) => c.itemKey !== itemKey);
		try {
			const transport = createClientTransport();
			await trackHomeAction(
				transport,
				"snooze",
				itemKey,
				JSON.stringify({ snooze_hours: hours }),
			);
		} catch {
			// Fire-and-forget — the recall rail must not surface transient
			// network failures as blocking errors. The next Home payload
			// refresh will reconcile state.
		}
	};

	const dismiss = async (itemKey: string) => {
		candidates = candidates.filter((c) => c.itemKey !== itemKey);
		try {
			const transport = createClientTransport();
			await trackHomeAction(transport, "dismiss_recall", itemKey);
		} catch {
			// Fire-and-forget.
		}
	};

	/** Inject initial candidates from Home response (single-fetch contract). */
	const setCandidates = (data: RecallCandidateData[]) => {
		candidates = data;
	};

	return {
		get candidates() {
			return candidates;
		},
		setCandidates,
		snooze,
		dismiss,
	};
}

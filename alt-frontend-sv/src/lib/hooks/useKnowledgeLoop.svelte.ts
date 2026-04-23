/**
 * useKnowledgeLoop — client-side state + actions for the /loop page.
 *
 * Invariant 28 (ADR-000831 §8 single-emission): this hook MUST NOT import
 * `$lib/connect/knowledge_home` / `trackHomeAction`. Loop UI owns its own
 * event emission lane. The static lint rule in biome.jsonc enforces this.
 *
 * Calls go through POST /loop/transition — never straight to alt-backend —
 * so the backend token wiring stays inside the SvelteKit server runtime.
 */

import { uuidv7 } from "$lib/utils/uuidv7";
import { canTransition } from "./loop-transitions";
import { makeObserveThrottle } from "./loop-observe-throttle";
import type {
	KnowledgeLoopEntryData,
	KnowledgeLoopResult,
	KnowledgeLoopSessionStateData,
	LoopStageName,
} from "$lib/connect/knowledge_loop";

type Trigger = "user_tap" | "dwell" | "keyboard" | "programmatic";

type TransitionResult =
	| { status: "accepted"; replay?: boolean }
	| { status: "forbidden"; reason: string }
	| { status: "stale" }
	| { status: "error"; message: string };

const OBSERVE_THROTTLE_MS = 60_000;

export interface UseKnowledgeLoopOptions {
	initial: KnowledgeLoopResult;
	lensModeId: string;
	fetchImpl?: typeof fetch;
}

export function useKnowledgeLoop(opts: UseKnowledgeLoopOptions) {
	const fetcher = opts.fetchImpl ?? fetch;
	let entries = $state<KnowledgeLoopEntryData[]>([...opts.initial.foregroundEntries]);
	let sessionState = $state<KnowledgeLoopSessionStateData | undefined>(
		opts.initial.sessionState,
	);
	let lastError = $state<string | null>(null);
	const inFlight = new SvelteSet();
	const observeThrottle = makeObserveThrottle(OBSERVE_THROTTLE_MS);

	function isInFlight(entryKey: string): boolean {
		return inFlight.has(entryKey);
	}

	function findEntry(entryKey: string): KnowledgeLoopEntryData | undefined {
		return entries.find((e) => e.entryKey === entryKey);
	}

	async function post(body: {
		clientTransitionId: string;
		entryKey: string;
		fromStage: LoopStageName;
		toStage: LoopStageName;
		trigger: Trigger;
	}): Promise<TransitionResult> {
		const entry = findEntry(body.entryKey);
		try {
			const res = await fetcher("/loop/transition", {
				method: "POST",
				headers: { "content-type": "application/json" },
				body: JSON.stringify({
					lensModeId: opts.lensModeId,
					observedProjectionRevision:
						entry?.projectionRevision ?? sessionState?.projectionRevision ?? 0,
					...body,
				}),
			});
			if (res.status === 200) {
				const json = (await res.json()) as { replay?: boolean };
				return { status: "accepted", replay: json.replay === true };
			}
			if (res.status === 409) return { status: "stale" };
			if (res.status === 400)
				return { status: "forbidden", reason: "invalid_body" };
			return { status: "error", message: `http_${res.status}` };
		} catch (e) {
			return {
				status: "error",
				message: e instanceof Error ? e.message : "network_error",
			};
		}
	}

	/**
	 * Emit `KnowledgeLoopObserved` once per 60-second window per entry.
	 * Returns true if the transition was actually posted.
	 */
	async function observe(entryKey: string): Promise<boolean> {
		if (!observeThrottle.shouldEmit(entryKey, Date.now())) return false;
		if (inFlight.has(entryKey)) return false;
		const entry = findEntry(entryKey);
		if (!entry || entry.proposedStage !== "observe") return false;

		inFlight.add(entryKey);
		try {
			const result = await post({
				clientTransitionId: uuidv7(),
				entryKey,
				fromStage: "observe",
				toStage: "orient",
				trigger: "dwell",
			});
			if (result.status === "accepted") {
				applyLocalStage(entryKey, "orient");
				return true;
			}
			if (result.status === "error" || result.status === "stale") {
				observeThrottle.reset(entryKey);
				lastError = result.status;
			}
			return false;
		} finally {
			inFlight.delete(entryKey);
		}
	}

	/**
	 * Perform a deliberate transition (Decide / Act / Return). Returns the result
	 * so the caller can react (e.g. open a URL when the server has accepted Act).
	 */
	async function transitionTo(
		entryKey: string,
		toStage: LoopStageName,
		trigger: Trigger = "user_tap",
	): Promise<TransitionResult> {
		const entry = findEntry(entryKey);
		if (!entry) return { status: "error", message: "unknown_entry" };

		const from = entry.proposedStage;
		if (!canTransition(from, toStage)) {
			return {
				status: "forbidden",
				reason: `cannot go ${from} → ${toStage}`,
			};
		}

		if (inFlight.has(entryKey)) {
			return { status: "forbidden", reason: "in_flight" };
		}

		inFlight.add(entryKey);
		try {
			const result = await post({
				clientTransitionId: uuidv7(),
				entryKey,
				fromStage: from,
				toStage,
				trigger,
			});
			if (result.status === "accepted") {
				applyLocalStage(entryKey, toStage);
			} else if (result.status === "error" || result.status === "stale") {
				lastError = result.status;
			}
			return result;
		} finally {
			inFlight.delete(entryKey);
		}
	}

	/**
	 * Optimistic dismiss. The UI fades the tile immediately; the server call is
	 * fire-and-forget for responsiveness. Network errors are logged to lastError
	 * but not re-surfaced as blocking UI.
	 */
	async function dismiss(entryKey: string): Promise<void> {
		const entry = findEntry(entryKey);
		if (!entry) return;
		applyLocalDismiss(entryKey);
		// Dismiss is expressed as decide→act with an intent hint on the UI side.
		// In the Loop event log it is a normal Act with target_type = "ask" /
		// "article" plus an optional "snooze" action id.
		const from = entry.proposedStage;
		const to: LoopStageName = canTransition(from, "act") ? "act" : from;
		if (to === from) return;

		try {
			await post({
				clientTransitionId: uuidv7(),
				entryKey,
				fromStage: from,
				toStage: to,
				trigger: "user_tap",
			});
		} catch {
			// optimistic: stay dismissed locally even if upstream reports an error
		}
	}

	function applyLocalStage(entryKey: string, to: LoopStageName) {
		entries = entries.map((e) =>
			e.entryKey === entryKey ? { ...e, proposedStage: to } : e,
		);
		if (sessionState) {
			sessionState = { ...sessionState, currentStage: to };
		}
	}

	function applyLocalDismiss(entryKey: string) {
		entries = entries.map((e) =>
			e.entryKey === entryKey ? { ...e, dismissState: "dismissed" } : e,
		);
	}

	return {
		get entries() {
			return entries;
		},
		get sessionState() {
			return sessionState;
		},
		get error() {
			return lastError;
		},
		observe,
		transitionTo,
		dismiss,
		canTransition,
		isInFlight,
	};
}

// Svelte 5 does not (yet) ship a reactive Set primitive; we only need
// membership checks for in-flight tracking, so a plain Set is fine here —
// button disabled state polls `isInFlight` on each user action.
class SvelteSet extends Set<string> {}

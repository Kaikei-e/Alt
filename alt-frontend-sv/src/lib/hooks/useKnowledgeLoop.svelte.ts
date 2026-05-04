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

import type {
	ActTargetTypeName,
	DecisionIntentName,
	KnowledgeLoopEntryData,
	KnowledgeLoopResult,
	KnowledgeLoopSessionStateData,
	LoopStageName,
} from "$lib/connect/knowledge_loop";
import { uuidv7 } from "$lib/utils/uuidv7";
import type { LoopStreamFrame } from "./loop-stream-frames";
import { canTransition } from "./loop-transitions";

type Trigger =
	| "user_tap"
	| "dwell"
	| "keyboard"
	| "programmatic"
	| "defer"
	| "recheck"
	| "archive"
	| "mark_reviewed";

export type ReviewAction = "recheck" | "archive" | "mark_reviewed";

export interface TransitionMetadata {
	presentedIntents?: DecisionIntentName[];
	actedIntent?: Exclude<DecisionIntentName, "unspecified">;
	actionId?: string;
	targetType?: Exclude<ActTargetTypeName, "unspecified">;
	targetRef?: string;
	continueFlag?: boolean;
}

type TransitionResult =
	| { status: "accepted"; replay?: boolean }
	| { status: "forbidden"; reason: string }
	| { status: "stale" }
	| { status: "rate_limited" }
	| { status: "error"; message: string };

/**
 * Per-call deadline for `/loop/transition` so a stalled fetch (server JWT
 * expiry mid-flight, network drop, bfcache-frozen await frame) cannot leave
 * `inFlight` populated forever. Exported for tests so they can advance fake
 * timers past it deterministically.
 */
export const TRANSITION_TIMEOUT_MS = 8_000;

export interface UseKnowledgeLoopOptions {
	initial: KnowledgeLoopResult;
	lensModeId: string;
	fetchImpl?: typeof fetch;
}

export function useKnowledgeLoop(opts: UseKnowledgeLoopOptions) {
	const fetcher = opts.fetchImpl ?? fetch;
	let entries = $state<KnowledgeLoopEntryData[]>([
		...opts.initial.foregroundEntries,
	]);
	let bucketEntries = $state<KnowledgeLoopEntryData[]>([
		...opts.initial.bucketEntries,
	]);
	let sessionState = $state<KnowledgeLoopSessionStateData | undefined>(
		opts.initial.sessionState,
	);
	let lastError = $state<string | null>(null);
	const inFlight = new SvelteSet();
	// Wall-clock stamp per inFlight key. Used by `replaceSnapshot` to gc keys
	// older than `TRANSITION_TIMEOUT_MS` so a pre-fix stalled fetch never
	// leaves `LoopEntryTile`'s `disabled={inFlight}` gate stuck on reload.
	const inFlightStartedAt = new Map<string, number>();
	// Tracks entry keys the user has optimistically dismissed locally. The
	// server-side projection lags the dismiss event by one projector tick, so
	// without this guard a stream-driven `replaceSnapshot` would briefly flash
	// the dismissed tile back into the foreground until the next snapshot.
	const optimisticallyDismissed = new Set<string>();
	const optimisticallyStaged = new Map<string, LoopStageName>();

	function isInFlight(entryKey: string): boolean {
		return inFlight.has(entryKey);
	}

	function findEntry(entryKey: string): KnowledgeLoopEntryData | undefined {
		return (
			entries.find((e) => e.entryKey === entryKey) ??
			bucketEntries.find((e) => e.entryKey === entryKey)
		);
	}

	function effectiveStage(entry: KnowledgeLoopEntryData): LoopStageName {
		return entry.currentEntryStage ?? entry.proposedStage;
	}

	async function post(body: {
		clientTransitionId: string;
		entryKey: string;
		fromStage: LoopStageName;
		toStage: LoopStageName;
		trigger: Trigger;
		metadata?: TransitionMetadata;
	}): Promise<TransitionResult> {
		const entry = findEntry(body.entryKey);
		// Per-call deadline so a stalled fetch (server JWT expiry mid-flight,
		// network drop, bfcache freeze) cannot leave the `finally` block — and
		// therefore `inFlight.delete(...)` — pending forever.
		const ac = new AbortController();
		const deadline = setTimeout(
			() => ac.abort(new DOMException("transition_timeout", "AbortError")),
			TRANSITION_TIMEOUT_MS,
		);
		try {
			const res = await fetcher("/loop/transition", {
				method: "POST",
				headers: { "content-type": "application/json" },
				signal: ac.signal,
				body: JSON.stringify({
					lensModeId: opts.lensModeId,
					observedProjectionRevision:
						entry?.projectionRevision ?? sessionState?.projectionRevision ?? 0,
					...body,
					...body.metadata,
					metadata: undefined,
				}),
			});
			if (res.status === 200) {
				const json = (await res.json()) as { replay?: boolean };
				return { status: "accepted", replay: json.replay === true };
			}
			if (res.status === 409) return { status: "stale" };
			if (res.status === 429) return { status: "rate_limited" };
			if (res.status === 400)
				return { status: "forbidden", reason: "invalid_body" };
			return { status: "error", message: `http_${res.status}` };
		} catch (e) {
			const isAbort = e instanceof DOMException && e.name === "AbortError";
			return {
				status: "error",
				message: isAbort
					? "transition_timeout"
					: e instanceof Error
						? e.message
						: "network_error",
			};
		} finally {
			clearTimeout(deadline);
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
		metadata?: TransitionMetadata,
		options: { optimistic?: boolean } = {},
	): Promise<TransitionResult> {
		const entry = findEntry(entryKey);
		if (!entry) return { status: "error", message: "unknown_entry" };

		const from = effectiveStage(entry);
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
		inFlightStartedAt.set(entryKey, Date.now());
		// Optimistic patch is opt-in. The tile's tap-to-expand gesture passes
		// `optimistic: true` so the OODA experience flips data-stage before the
		// BFF reply lands. Other transitionTo callers (workspace pipeline,
		// advanceEntry) keep the conservative "apply on accepted" semantics
		// pre-existing tests rely on.
		const previousStage = effectiveStage(entry);
		if (options.optimistic) {
			applyLocalStage(entryKey, toStage);
		}
		try {
			const result = await post({
				clientTransitionId: uuidv7(),
				entryKey,
				fromStage: from,
				toStage,
				trigger,
				metadata,
			});
			if (result.status === "accepted") {
				if (!options.optimistic) {
					applyLocalStage(entryKey, toStage);
				}
			} else if (
				result.status === "error" ||
				result.status === "stale" ||
				result.status === "rate_limited" ||
				result.status === "forbidden"
			) {
				lastError =
					result.status === "forbidden" ? "forbidden" : result.status;
				if (options.optimistic) {
					// Revert when the server refused after we already flipped.
					applyLocalStage(entryKey, previousStage);
				}
			}
			return result;
		} finally {
			inFlight.delete(entryKey);
			inFlightStartedAt.delete(entryKey);
		}
	}

	/**
	 * Optimistic dismiss. The UI fades the tile immediately; the server call is
	 * fire-and-forget for responsiveness. Network errors are logged to lastError
	 * but not re-surfaced as blocking UI.
	 *
	 * Dismiss routes to `KnowledgeLoopDeferred` (canonical contract §8.2 — soft
	 * dismiss / snooze). Same-stage transition (`fromStage === toStage`) with
	 * the `defer` trigger is the only shape the BFF + classifier allows for
	 * this event; the projector flips `dismiss_state` to `deferred` and the
	 * read filter then excludes the row from the foreground on next reload.
	 *
	 * Pre-fix: dismiss only fired the network call when the entry was already
	 * in the `decide` stage (the only stage from which `decide → act` was a
	 * legal OODA transition). Every other stage early-returned silently, so
	 * `optimisticallyDismissed` was the only thing keeping the tile out of view
	 * — wiped on reload, hence "dismissed tiles came back".
	 */
	async function dismiss(entryKey: string): Promise<void> {
		const entry = findEntry(entryKey);
		if (!entry) return;
		const stage = effectiveStage(entry);
		applyLocalDismiss(entryKey);

		try {
			await post({
				clientTransitionId: uuidv7(),
				entryKey,
				fromStage: stage,
				toStage: stage,
				trigger: "defer",
			});
		} catch {
			// optimistic: stay dismissed locally even if upstream reports an error
		}
	}

	/**
	 * Review-lane re-evaluation (fb.md §F). Recheck re-surfaces the entry as
	 * NOW with fresh `freshness_at`; archive permanently dismisses; mark
	 * reviewed acknowledges without re-surfacing unless new evidence arrives.
	 *
	 * Same-stage transition (`fromStage === toStage`) like dismiss — the
	 * OODA stage doesn't move, only `dismiss_state` does. The projector
	 * reads the trigger to choose between recheck (re-arms the entry),
	 * archive, and mark-reviewed.
	 *
	 * Optimistic UI: archive / mark_reviewed remove the entry locally
	 * immediately; recheck leaves the entry visible (it'll re-render at
	 * the next `replaceSnapshot` once the projector has caught up).
	 */
	async function reviewAction(
		entryKey: string,
		action: ReviewAction,
	): Promise<TransitionResult> {
		const entry = findEntry(entryKey);
		if (!entry) return { status: "error", message: "unknown_entry" };
		if (inFlight.has(entryKey)) {
			return { status: "forbidden", reason: "in_flight" };
		}

		// Archive and mark-reviewed pull the entry out of view immediately —
		// just like dismiss does — so the foreground reflects the user's
		// decision before the projector catches up.
		if (action === "archive" || action === "mark_reviewed") {
			applyLocalDismiss(entryKey);
		}

		inFlight.add(entryKey);
		inFlightStartedAt.set(entryKey, Date.now());
		try {
			return await post({
				clientTransitionId: uuidv7(),
				entryKey,
				fromStage: effectiveStage(entry),
				toStage: effectiveStage(entry),
				trigger: action,
				metadata: { actionId: action },
			});
		} finally {
			inFlight.delete(entryKey);
			inFlightStartedAt.delete(entryKey);
		}
	}

	function applyLocalStage(entryKey: string, to: LoopStageName) {
		optimisticallyStaged.set(entryKey, to);
		entries = entries.map((e) =>
			e.entryKey === entryKey ? { ...e, currentEntryStage: to } : e,
		);
		bucketEntries = bucketEntries.map((e) =>
			e.entryKey === entryKey ? { ...e, currentEntryStage: to } : e,
		);
		if (sessionState) {
			sessionState = { ...sessionState, currentStage: to };
		}
	}

	function applyLocalDismiss(entryKey: string) {
		// Remove the entry from the foreground array so Svelte's keyed `#each`
		// detects a leaver and plays the parent-level `out:` transition +
		// `animate:flip` for survivors. The pre-fix path mutated `dismissState`
		// in place, which left the row in the DOM with a `.dismissing` class
		// collapsing `max-height` — combined with the fetch-storm starving the
		// main thread of rAFs, neighbors visibly overlapped mid-collapse.
		optimisticallyDismissed.add(entryKey);
		entries = entries.filter((e) => e.entryKey !== entryKey);
		bucketEntries = bucketEntries.filter((e) => e.entryKey !== entryKey);
	}

	function applyInlineEntry(entry: KnowledgeLoopEntryData) {
		const foregroundWithout = entries.filter(
			(e) => e.entryKey !== entry.entryKey,
		);
		const bucketWithout = bucketEntries.filter(
			(e) => e.entryKey !== entry.entryKey,
		);
		if (entry.surfaceBucket === "now") {
			entries = applyOptimisticOverlays([entry, ...foregroundWithout]);
			bucketEntries = applyOptimisticOverlays(bucketWithout);
			return;
		}
		entries = applyOptimisticOverlays(foregroundWithout);
		bucketEntries = applyOptimisticOverlays([entry, ...bucketWithout]);
	}

	function applyStreamFrame(frame: LoopStreamFrame): boolean {
		switch (frame.kind) {
			case "appended":
			case "revised":
				if (!frame.inlineEntry) return false;
				applyInlineEntry(frame.inlineEntry);
				return true;
			case "withdrawn":
				applyLocalDismiss(frame.entryKey);
				return true;
			case "superseded":
				// Remove old entry immediately; the replacement arrives via a
				// subsequent "appended" frame with an inlineEntry.
				entries = entries.filter((e) => e.entryKey !== frame.entryKey);
				bucketEntries = bucketEntries.filter(
					(e) => e.entryKey !== frame.entryKey,
				);
				return true;
			default:
				return false;
		}
	}

	function applyOptimisticOverlays(
		list: KnowledgeLoopEntryData[],
	): KnowledgeLoopEntryData[] {
		return list
			.filter((e) => !optimisticallyDismissed.has(e.entryKey))
			.map((e) => {
				const stage = optimisticallyStaged.get(e.entryKey);
				return stage ? { ...e, currentEntryStage: stage } : e;
			});
	}

	function replaceSnapshot(next: KnowledgeLoopResult) {
		// Stream-driven coalesced refresh path. Re-seed entries + sessionState
		// from a freshly fetched GetKnowledgeLoop result without forcing an SSR
		// `__data.json` refetch. Optimistically dismissed entries stay removed
		// until the projection catches up and the server itself stops returning
		// them.
		entries = applyOptimisticOverlays(next.foregroundEntries);
		bucketEntries = applyOptimisticOverlays(next.bucketEntries);
		sessionState = next.sessionState;
		const stillReturned = new Set(
			[...next.foregroundEntries, ...next.bucketEntries].map((e) => e.entryKey),
		);
		const returnedByKey = new Map(
			[...next.foregroundEntries, ...next.bucketEntries].map((e) => [
				e.entryKey,
				e,
			]),
		);
		// Garbage-collect dismissals the server has acknowledged: any key the
		// new snapshot already omits no longer needs the local guard.
		for (const key of [...optimisticallyDismissed]) {
			if (!stillReturned.has(key)) optimisticallyDismissed.delete(key);
		}
		for (const [key, stage] of [...optimisticallyStaged]) {
			const returned = returnedByKey.get(key);
			if (!returned || returned.currentEntryStage === stage) {
				optimisticallyStaged.delete(key);
			}
		}
		// Garbage-collect stale `inFlight` keys. Two ways an entry becomes
		// stale: (a) the snapshot no longer contains it (server moved past it
		// while we were waiting), or (b) its start timestamp is older than the
		// per-call deadline (the awaited fetch never resolved — bfcache freeze,
		// JWT expiry, dropped connection — so the `try/finally` never ran).
		// Without this gc, `LoopEntryTile`'s `disabled={inFlight}` gate would
		// stay locked permanently after a long idle.
		const now = Date.now();
		for (const key of [...inFlight]) {
			const startedAt = inFlightStartedAt.get(key);
			const tooOld =
				startedAt !== undefined && now - startedAt > TRANSITION_TIMEOUT_MS;
			if (!stillReturned.has(key) || tooOld) {
				inFlight.delete(key);
				inFlightStartedAt.delete(key);
			}
		}
	}

	/**
	 * Clears every tracked in-flight key. Called by the page when
	 * `loop-visibility-recovery` detects the user has returned from a long
	 * background period — at which point any pending `/loop/transition`
	 * request is functionally lost (server JWT expired, network reset) and
	 * gating button clicks on it would lock the UI indefinitely.
	 */
	function resetInFlight(_reason: "snapshot" | "visibility" | "timeout") {
		inFlight.clear();
		inFlightStartedAt.clear();
	}

	return {
		get entries() {
			return entries;
		},
		get bucketEntries() {
			return bucketEntries;
		},
		get sessionState() {
			return sessionState;
		},
		get error() {
			return lastError;
		},
		transitionTo,
		dismiss,
		reviewAction,
		replaceSnapshot,
		applyStreamFrame,
		resetInFlight,
		canTransition,
		isInFlight,
	};
}

// Svelte 5 does not (yet) ship a reactive Set primitive; we only need
// membership checks for in-flight tracking, so a plain Set is fine here —
// button disabled state polls `isInFlight` on each user action.
class SvelteSet extends Set<string> {}

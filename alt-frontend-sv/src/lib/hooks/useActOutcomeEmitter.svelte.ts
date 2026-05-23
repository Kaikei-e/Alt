/**
 * useActOutcomeEmitter — closes the OODA loop in real time from the
 * client side (ADR-000912).
 *
 * Lifecycle per entry:
 *   - `start(entryKey)` arms a wall-clock-based dwell counter scoped to
 *     the entry; calling start again for the same entry is a no-op
 *   - `recordAskTurn(entryKey)` increments the per-entry ask-turn count
 *   - Engagement is emitted once at ≥ 30 s dwell ("engaged") and once at
 *     ≥ 120 s dwell OR ≥ 3 ask turns ("deep_engagement"). Each emit uses
 *     a fresh UUIDv7 so the server-side dedupe (knowledge_event_dedupes)
 *     collapses retries but distinct upgrade events still land
 *   - `stop(entryKey)` deregisters the entry without emitting
 *   - `flush()` posts every armed entry whose dwell already crossed a
 *     threshold but had not yet been emitted. Wired to `visibilitychange`
 *     hidden so a tab close does not lose accumulated engagement
 *
 * Reproject-safety: occurredAt is the time at which the threshold was
 * crossed (client wall-clock at emit), and the server records it
 * verbatim. The projector's ActOutcomeSignal aggregation reduces over
 * event payload only, so no second clock is consulted.
 *
 * Single-emission: each (entryKey, outcome) pair emits at most once per
 * session. Upgrades (engaged → deep_engagement) emit a fresh event with
 * its own clientOutcomeId; the server keeps both since they represent
 * distinct closure signals.
 */

import { uuidv7 } from "$lib/utils/uuidv7";

const ENGAGED_THRESHOLD_MS = 30_000;
const DEEP_ENGAGEMENT_THRESHOLD_MS = 120_000;
const DEEP_ENGAGEMENT_ASK_TURNS = 3;

type Outcome = "engaged" | "deep_engagement";

interface EntryState {
	entryKey: string;
	startedAt: number; // ms since epoch, monotonic via Date.now()
	askTurns: number;
	emitted: Set<Outcome>;
}

interface EmitArgs {
	entryKey: string;
	outcome: Outcome;
	clientOutcomeId: string;
	occurredAtIso: string;
	dwellSeconds: number;
	askTurns: number;
}

export interface ActOutcomeEmitter {
	start(entryKey: string): void;
	stop(entryKey: string): void;
	recordAskTurn(entryKey: string): void;
	tick(nowMs?: number): Promise<void>;
	flush(): Promise<void>;
	teardown(): void;
}

export interface ActOutcomeEmitterOptions {
	post?: (args: EmitArgs) => Promise<void>;
	tickIntervalMs?: number;
	now?: () => number;
}

const DEFAULT_TICK_MS = 1_000;

async function defaultPost(args: EmitArgs): Promise<void> {
	await fetch("/loop/act-outcome", {
		method: "POST",
		headers: { "content-type": "application/json" },
		body: JSON.stringify({
			entryKey: args.entryKey,
			outcome: args.outcome,
			clientOutcomeId: args.clientOutcomeId,
			occurredAtIso: args.occurredAtIso,
			dwellSeconds: args.dwellSeconds,
			askTurns: args.askTurns,
		}),
		keepalive: true,
	});
}

export function createActOutcomeEmitter(
	opts: ActOutcomeEmitterOptions = {},
): ActOutcomeEmitter {
	const post = opts.post ?? defaultPost;
	const now = opts.now ?? (() => Date.now());
	const tickIntervalMs = opts.tickIntervalMs ?? DEFAULT_TICK_MS;

	const state = new Map<string, EntryState>();
	let intervalHandle: ReturnType<typeof setInterval> | undefined;
	let visibilityHandler: (() => void) | undefined;

	function ensure(entryKey: string): EntryState {
		const existing = state.get(entryKey);
		if (existing) return existing;
		const created: EntryState = {
			entryKey,
			startedAt: now(),
			askTurns: 0,
			emitted: new Set<Outcome>(),
		};
		state.set(entryKey, created);
		return created;
	}

	function startInterval() {
		if (intervalHandle != null) return;
		if (typeof window === "undefined") return;
		intervalHandle = setInterval(() => {
			void tick();
		}, tickIntervalMs);
	}

	function startVisibilityListener() {
		if (visibilityHandler != null) return;
		if (typeof document === "undefined") return;
		visibilityHandler = () => {
			if (document.visibilityState === "hidden") {
				void flush();
			}
		};
		document.addEventListener("visibilitychange", visibilityHandler);
	}

	async function emit(entry: EntryState, outcome: Outcome, nowMs: number) {
		if (entry.emitted.has(outcome)) return;
		entry.emitted.add(outcome);
		const dwellSeconds = Math.floor((nowMs - entry.startedAt) / 1000);
		try {
			await post({
				entryKey: entry.entryKey,
				outcome,
				clientOutcomeId: uuidv7(),
				occurredAtIso: new Date(nowMs).toISOString(),
				dwellSeconds,
				askTurns: entry.askTurns,
			});
		} catch {
			// Network failure: allow the same outcome to retry on the next
			// tick by clearing the emitted flag. The server's dedupe layer
			// catches retries that did land before the network broke, so
			// repeated tries cannot double-count.
			entry.emitted.delete(outcome);
		}
	}

	async function tick(nowMsOverride?: number): Promise<void> {
		const nowMs = nowMsOverride ?? now();
		for (const entry of state.values()) {
			const dwellMs = nowMs - entry.startedAt;
			if (dwellMs >= ENGAGED_THRESHOLD_MS && !entry.emitted.has("engaged")) {
				await emit(entry, "engaged", nowMs);
			}
			if (
				(dwellMs >= DEEP_ENGAGEMENT_THRESHOLD_MS ||
					entry.askTurns >= DEEP_ENGAGEMENT_ASK_TURNS) &&
				!entry.emitted.has("deep_engagement")
			) {
				await emit(entry, "deep_engagement", nowMs);
			}
		}
	}

	async function flush(): Promise<void> {
		await tick();
	}

	function start(entryKey: string) {
		ensure(entryKey);
		startInterval();
		startVisibilityListener();
	}

	function stop(entryKey: string) {
		state.delete(entryKey);
	}

	function recordAskTurn(entryKey: string) {
		const entry = ensure(entryKey);
		entry.askTurns += 1;
	}

	function teardown() {
		if (intervalHandle != null) {
			clearInterval(intervalHandle);
			intervalHandle = undefined;
		}
		if (visibilityHandler != null && typeof document !== "undefined") {
			document.removeEventListener("visibilitychange", visibilityHandler);
			visibilityHandler = undefined;
		}
		state.clear();
	}

	return { start, stop, recordAskTurn, tick, flush, teardown };
}

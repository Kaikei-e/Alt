/**
 * useKnowledgeLoopStream — Knowledge Loop subscription hook.
 *
 * Drives the server-streaming StreamKnowledgeLoopUpdates RPC, classifies frames
 * via the pure `classifyLoopStreamFrame` helper, and exposes a small reactive
 * surface the /loop page can react to. Heartbeat frames advance the local
 * projection_seq_hiwater without triggering UI events.
 *
 * Design notes:
 *  - Browser → alt-backend direct: Svelte 5 uses the client transport and
 *    relies on cookies already being attached (BFF hand-off is not in the
 *    stream path). This mirrors useStreamUpdates for Knowledge Home.
 *  - ADR-000831 §3.3 (reproject-safe): this hook is a pure consumer; it never
 *    writes back to projection state.
 *  - MVP: no multi-tab leader election. If two tabs each subscribe, each gets
 *    its own stream; backend rate-limit protects against abuse.
 */

import { createClient } from "@connectrpc/connect";
import { untrack } from "svelte";
import { createClientTransport } from "$lib/connect/transport-client";
import { KnowledgeLoopService } from "$lib/gen/alt/knowledge/loop/v1/knowledge_loop_pb";
import {
	classifyLoopStreamFrame,
	type LoopStreamFrame,
} from "./loop-stream-frames";

const BASE_RETRY_DELAY_MS = 1000;
const MAX_RETRY_DELAY_MS = 15_000;
const MAX_RETRIES = 10;
const CURSOR_STORAGE_PREFIX = "knowledge-loop:resume:";

export interface UseKnowledgeLoopStreamOptions {
	/** Toggle the subscription on/off reactively (e.g. page unmount, flag off). */
	get enabled(): boolean;
	/** Server-scoped lens id; must match the GetKnowledgeLoop request. */
	get lensModeId(): string;
	/** Called for every non-heartbeat frame; the hook does not mutate entries. */
	onFrame?: (frame: LoopStreamFrame) => void;
	/**
	 * Called when the server sends a terminal `stream_expired` envelope (JWT exp,
	 * idle timeout, etc). The UI typically refetches via GetKnowledgeLoop here so
	 * the next reconnect starts from a fresh snapshot.
	 */
	onExpired?: (reason: string) => void | Promise<void>;
	/**
	 * Persist the SSE resume cursor across hook remounts (navigation to article
	 * reader and back, SvelteKit invalidateAll). Set per (user, lensMode) so a
	 * tab returning to /loop resumes from the last delivered projection seq
	 * instead of replaying from zero. Falls back to in-memory state when
	 * undefined or when sessionStorage is unavailable.
	 */
	cursorPersistKey?: string;
}

function loadPersistedCursor(key: string | undefined): bigint {
	if (!key) return 0n;
	try {
		const raw = sessionStorage.getItem(CURSOR_STORAGE_PREFIX + key);
		if (!raw) return 0n;
		const parsed = BigInt(raw);
		return parsed > 0n ? parsed : 0n;
	} catch {
		return 0n;
	}
}

function savePersistedCursor(key: string | undefined, value: bigint): void {
	if (!key) return;
	try {
		sessionStorage.setItem(CURSOR_STORAGE_PREFIX + key, value.toString());
	} catch {
		// sessionStorage may be unavailable (privacy mode, server-side); ignore.
	}
}

function clearPersistedCursor(key: string | undefined): void {
	if (!key) return;
	try {
		sessionStorage.removeItem(CURSOR_STORAGE_PREFIX + key);
	} catch {
		// ignore
	}
}

export function useKnowledgeLoopStream(opts: UseKnowledgeLoopStreamOptions) {
	let isConnected = $state(false);
	let lastSeqHiwater = $state<bigint>(loadPersistedCursor(opts.cursorPersistKey));
	let retryCount = $state(0);
	let lastError = $state<string | null>(null);

	let abortController: AbortController | null = null;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let stopped = false;

	function bumpCursor(next: bigint) {
		if (next <= lastSeqHiwater) return;
		lastSeqHiwater = next;
		savePersistedCursor(opts.cursorPersistKey, next);
	}

	async function connect() {
		if (stopped || retryCount >= MAX_RETRIES) return;

		const myAbort = new AbortController();
		abortController = myAbort;
		try {
			const transport = createClientTransport();
			const client = createClient(KnowledgeLoopService, transport);

			const stream = client.streamKnowledgeLoopUpdates(
				{
					lensModeId: opts.lensModeId,
					resumeFromSeq: lastSeqHiwater,
				},
				{ signal: myAbort.signal },
			);

			isConnected = true;
			lastError = null;

			for await (const msg of stream) {
				const frame = classifyLoopStreamFrame(msg);
				if (!frame) continue;

				// Heartbeat advances the cursor silently.
				if (frame.kind === "heartbeat") {
					bumpCursor(frame.projectionSeqHiwater);
					continue;
				}

				// Terminal envelope — server requested we reconnect fresh.
				if (frame.kind === "expired") {
					isConnected = false;
					try {
						await opts.onExpired?.(frame.reason);
					} catch {
						// onExpired failure should not block reconnect.
					}
					// Server replays from scratch on a fresh stream; drop persisted
					// cursor so the reconnect does not skip events the server already
					// committed to replay.
					lastSeqHiwater = 0n;
					clearPersistedCursor(opts.cursorPersistKey);
					retryCount = 0;
					scheduleReconnect(BASE_RETRY_DELAY_MS);
					return;
				}

				bumpCursor(frame.projectionSeqHiwater);

				try {
					opts.onFrame?.(frame);
				} catch {
					// Handler errors are UI concerns, not stream concerns.
				}
				retryCount = 0;
			}

			// Stream ended without terminal envelope — server closed.
			isConnected = false;
			if (!stopped && !myAbort.signal.aborted) scheduleReconnect();
		} catch (err) {
			isConnected = false;
			lastError = err instanceof Error ? err.message : "stream_error";
			// If this connect was aborted by *our own* disconnect/effect cleanup,
			// the new effect run (or an explicit stop) has already taken
			// ownership. Re-scheduling here is the ghost-reconnect that produced
			// overlapping SSE sessions in production.
			if (myAbort.signal.aborted) return;
			if (!stopped) scheduleReconnect();
		}
	}

	function scheduleReconnect(delayOverride?: number) {
		if (reconnectTimer || stopped) return;
		const delay =
			delayOverride ??
			Math.min(BASE_RETRY_DELAY_MS * 2 ** retryCount, MAX_RETRY_DELAY_MS);
		reconnectTimer = setTimeout(() => {
			reconnectTimer = null;
			retryCount += 1;
			void connect();
		}, delay);
	}

	function disconnect() {
		stopped = true;
		if (abortController) {
			abortController.abort();
			abortController = null;
		}
		if (reconnectTimer) {
			clearTimeout(reconnectTimer);
			reconnectTimer = null;
		}
		isConnected = false;
	}

	// Track lens id by *value*, not by the surrounding `data` reference. Without
	// this guard, an `invalidateAll()` triggered elsewhere on the page replaces
	// `data` (a fresh prop reference each time), the effect's `data`-tracked
	// dependency churns, and the cleanup tears down + re-opens the stream — the
	// positive-feedback loop that produced the lockstep `stream_jwt_expired` log
	// waves in production. `untrack` keeps the deep read out of the dependency
	// graph; we feed lens id through a $derived value-equality gate instead.
	const trackedLensId = $derived.by(() => opts.lensModeId);

	$effect(() => {
		const enabled = opts.enabled;
		// trackedLensId is a $derived, so the effect re-runs only on value change.
		const _lens = trackedLensId;
		if (!enabled) return;

		// Use untrack for the imperative connect/disconnect side so any reactive
		// reads inside `connect()` (rare, but cheap insurance) do not subscribe
		// the effect to additional state.
		untrack(() => {
			stopped = false;
			retryCount = 0;
			void connect();
		});

		return () => {
			disconnect();
		};
	});

	return {
		get isConnected() {
			return isConnected;
		},
		get lastSeqHiwater() {
			return lastSeqHiwater;
		},
		get lastError() {
			return lastError;
		},
	};
}

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
}

export function useKnowledgeLoopStream(opts: UseKnowledgeLoopStreamOptions) {
	let isConnected = $state(false);
	let lastSeqHiwater = $state<bigint>(0n);
	let retryCount = $state(0);
	let lastError = $state<string | null>(null);

	let abortController: AbortController | null = null;
	let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
	let stopped = false;

	async function connect() {
		if (stopped || retryCount >= MAX_RETRIES) return;

		abortController = new AbortController();
		try {
			const transport = createClientTransport();
			const client = createClient(KnowledgeLoopService, transport);

			const stream = client.streamKnowledgeLoopUpdates(
				{
					lensModeId: opts.lensModeId,
					resumeFromSeq: lastSeqHiwater,
				},
				{ signal: abortController.signal },
			);

			isConnected = true;
			lastError = null;

			for await (const msg of stream) {
				const frame = classifyLoopStreamFrame(msg);
				if (!frame) continue;

				// Heartbeat advances the cursor silently.
				if (frame.kind === "heartbeat") {
					if (frame.projectionSeqHiwater > lastSeqHiwater) {
						lastSeqHiwater = frame.projectionSeqHiwater;
					}
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
					// Reset seq cursor because server will replay from scratch.
					lastSeqHiwater = 0n;
					retryCount = 0;
					scheduleReconnect(BASE_RETRY_DELAY_MS);
					return;
				}

				if (frame.projectionSeqHiwater > lastSeqHiwater) {
					lastSeqHiwater = frame.projectionSeqHiwater;
				}

				try {
					opts.onFrame?.(frame);
				} catch {
					// Handler errors are UI concerns, not stream concerns.
				}
				retryCount = 0;
			}

			// Stream ended without terminal envelope — server closed.
			isConnected = false;
			if (!stopped) scheduleReconnect();
		} catch (err) {
			isConnected = false;
			lastError = err instanceof Error ? err.message : "stream_error";
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

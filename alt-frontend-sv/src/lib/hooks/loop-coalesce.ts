/**
 * Coalesced refresh for the Knowledge Loop stream.
 *
 * Production regression (live nginx + alt-butterfly-facade + alt-backend logs,
 * 2026-04-26 04:30 UTC): the page-level `useKnowledgeLoopStream` callbacks
 * fired `invalidateAll()` unconditionally for every non-heartbeat frame and
 * every JWT expiry. With dozens of stream frames arriving per second, the
 * browser hit `ERR_INSUFFICIENT_RESOURCES` against the per-origin connection
 * ceiling and the page froze.
 *
 * This helper enforces three invariants:
 *
 *   1. Trailing-edge debounce — bursts within `windowMs` collapse into one
 *      call after the last trigger settles.
 *   2. Single-flight — while a refresh is in flight, additional triggers
 *      collapse into exactly one trailing follow-up that runs after the
 *      current call settles.
 *   3. Cancellable — `dispose()` cancels both the pending timer and any
 *      trailing follow-up. In-flight calls cannot be aborted (the caller
 *      owns the underlying async fn), but their settle is ignored after
 *      dispose.
 *
 * The helper is a pure factory (no Svelte runes), so it is exhaustively
 * unit-testable with `vi.useFakeTimers`.
 */

export interface CoalescedRefreshOptions {
	/** Debounce window in milliseconds. Default: 600. */
	windowMs?: number;
}

export interface CoalescedRefresh {
	/** Schedule a refresh respecting debounce + single-flight rules. */
	trigger(): void;
	/** Cancel any pending timer / trailing follow-up. Idempotent. */
	dispose(): void;
}

const DEFAULT_WINDOW_MS = 600;

export function makeCoalescedRefresh(
	refresh: () => Promise<void>,
	opts: CoalescedRefreshOptions = {},
): CoalescedRefresh {
	const windowMs = opts.windowMs ?? DEFAULT_WINDOW_MS;

	let pendingTimer: ReturnType<typeof setTimeout> | null = null;
	let inFlight = false;
	let trailingPending = false;
	let disposed = false;

	function clearPendingTimer() {
		if (pendingTimer !== null) {
			clearTimeout(pendingTimer);
			pendingTimer = null;
		}
	}

	function scheduleDebouncedFlight() {
		clearPendingTimer();
		pendingTimer = setTimeout(() => {
			pendingTimer = null;
			if (disposed) return;
			void runFlight();
		}, windowMs);
	}

	async function runFlight() {
		if (disposed) return;
		inFlight = true;
		try {
			await refresh();
		} catch {
			// Swallow caller errors — the coalescer must remain usable for the
			// next trigger. Caller is responsible for telemetry inside `refresh`.
		}
		// Do not put this in `finally`: a `return` inside finally would mask
		// any thrown control flow from the try block. The catch above already
		// guarantees we reach this line.
		inFlight = false;
		if (disposed) return;
		if (trailingPending) {
			trailingPending = false;
			scheduleDebouncedFlight();
		}
	}

	return {
		trigger() {
			if (disposed) return;
			if (inFlight) {
				// While a refresh is in flight, collapse all further triggers into
				// a single trailing follow-up. This is the single-flight guard.
				trailingPending = true;
				return;
			}
			scheduleDebouncedFlight();
		},
		dispose() {
			disposed = true;
			trailingPending = false;
			clearPendingTimer();
		},
	};
}

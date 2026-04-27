/**
 * Tab-return recovery for the Knowledge Loop hook.
 *
 * When the tab is backgrounded for longer than `thresholdMs`, an in-flight
 * `/loop/transition` request can stall (server JWT expiry mid-flight, network
 * dropped, bfcache freeze suspending the await frame). The hook's own
 * `try/finally { inFlight.delete(...) }` then never fires and `LoopEntryTile`
 * stays disabled forever.
 *
 * This factory wires `visibilitychange` (regular foreground/background) and
 * `pageshow` (bfcache restore) listeners so the page can clear stale in-flight
 * tracking on tab return.
 *
 * Quick blurs under the threshold are intentionally ignored: alt-tab cycles
 * are common and we don't want to clear in-flight state for fetches that are
 * about to resolve normally.
 */

export type RecoverReason = "visibility" | "bfcache";

export interface VisibilityRecoveryOptions {
	thresholdMs: number;
	onRecover: (info: { reason: RecoverReason }) => void;
	/** Injected for tests. Defaults to `Date.now`. */
	getNow?: () => number;
	/**
	 * Injected for tests. Defaults to the global `document`. The factory binds
	 * `addEventListener` / `removeEventListener` and reads `visibilityState`.
	 */
	document?: Document;
}

export interface VisibilityRecoveryHandle {
	dispose(): void;
}

export function startVisibilityRecovery(
	opts: VisibilityRecoveryOptions,
): VisibilityRecoveryHandle {
	const doc = opts.document ?? globalThis.document;
	const getNow = opts.getNow ?? Date.now;
	const thresholdMs = opts.thresholdMs;

	let hiddenAt: number | null = null;

	const onVisibilityChange = () => {
		if (doc.visibilityState === "hidden") {
			hiddenAt = getNow();
			return;
		}
		// visibilityState === "visible" (or any non-hidden state)
		if (hiddenAt === null) return;
		const elapsed = getNow() - hiddenAt;
		hiddenAt = null;
		if (elapsed > thresholdMs) {
			opts.onRecover({ reason: "visibility" });
		}
	};

	const onPageShow = (e: Event) => {
		// PageTransitionEvent in browsers; structural-typed for tests too.
		const persisted = (e as { persisted?: boolean }).persisted === true;
		if (!persisted) return;
		opts.onRecover({ reason: "bfcache" });
	};

	doc.addEventListener("visibilitychange", onVisibilityChange);
	doc.addEventListener("pageshow", onPageShow);

	return {
		dispose() {
			doc.removeEventListener("visibilitychange", onVisibilityChange);
			doc.removeEventListener("pageshow", onPageShow);
		},
	};
}

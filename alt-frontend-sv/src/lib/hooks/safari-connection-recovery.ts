/**
 * Safari Connection Recovery Hook
 *
 * Safari aggressively drops network connections when tabs are backgrounded
 * for power-saving. This hook detects:
 * 1. Extended background periods via visibilitychange
 * 2. bfcache restore via pageshow
 * 3. Network reconnection via online event
 *
 * When any of these conditions are met after prolonged inactivity, the
 * onRecoveryNeeded callback fires so the app can invalidate TanStack Query
 * caches and refetch stale data.
 *
 * Safari-specific behavior:
 * - NSURLErrorDomain -1004: "Could not connect to server" after background
 * - WebSocket connections silently dropped when tab loses focus
 * - fetch requests may fail or stall after bfcache restore
 */

export type RecoveryReason = "visibility" | "bfcache" | "online";

export interface RecoveryInfo {
	reason: RecoveryReason;
	hiddenDurationMs?: number;
}

export interface SafariConnectionRecoveryOptions {
	/** Minimum background duration (ms) before triggering recovery. Default 30s. */
	thresholdMs: number;
	/** Callback when recovery is needed. */
	onRecoveryNeeded: (info: RecoveryInfo) => void;
	/** Injected for tests. Defaults to Date.now. */
	getNow?: () => number;
	/** Injected for tests. Defaults to globalThis.document. */
	document?: Document;
	/** Injected for tests. Defaults to globalThis.navigator. */
	navigator?: Navigator;
	/** Injected for tests. Defaults to globalThis.window. */
	window?: Window;
}

export interface SafariConnectionRecoveryHandle {
	dispose(): void;
}

export function createSafariConnectionRecovery(
	opts: SafariConnectionRecoveryOptions,
): SafariConnectionRecoveryHandle {
	const doc = opts.document ?? globalThis.document;
	const nav = opts.navigator ?? globalThis.navigator;
	const win = opts.window ?? globalThis.window;
	const getNow = opts.getNow ?? Date.now;
	const thresholdMs = opts.thresholdMs;

	let hiddenAt: number | null = null;

	const onVisibilityChange = () => {
		if (doc.visibilityState === "hidden") {
			hiddenAt = getNow();
			return;
		}
		if (hiddenAt === null) return;
		const elapsed = getNow() - hiddenAt;
		hiddenAt = null;
		if (elapsed > thresholdMs) {
			opts.onRecoveryNeeded({
				reason: "visibility",
				hiddenDurationMs: elapsed,
			});
		}
	};

	const onPageShow = (e: Event) => {
		const persisted = (e as { persisted?: boolean }).persisted === true;
		if (!persisted) return;
		opts.onRecoveryNeeded({ reason: "bfcache", hiddenDurationMs: undefined });
	};

	const onOnline = () => {
		opts.onRecoveryNeeded({ reason: "online", hiddenDurationMs: undefined });
	};

	doc.addEventListener("visibilitychange", onVisibilityChange);
	doc.addEventListener("pageshow", onPageShow);
	win?.addEventListener("online", onOnline);

	return {
		dispose() {
			doc.removeEventListener("visibilitychange", onVisibilityChange);
			doc.removeEventListener("pageshow", onPageShow);
			win?.removeEventListener("online", onOnline);
		},
	};
}

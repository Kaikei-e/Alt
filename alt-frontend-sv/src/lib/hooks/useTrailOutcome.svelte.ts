import type { Transport } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { onDestroy } from "svelte";
import { browser } from "$app/environment";
import { beforeNavigate } from "$app/navigation";
import { base } from "$app/paths";
import { emitTrailOutcome } from "$lib/connect/knowledge_trail";
import { createClientTransport } from "$lib/connect/transport-client";

/**
 * createDwellTracker accumulates visible time and guarantees a single flush.
 * Framework-free so the accounting is unit-testable; useTrailOutcome wires it
 * to Page Visibility and navigation lifecycles.
 */
export function createDwellTracker(
	now: () => number = () => performance.now(),
) {
	let visibleSince: number | null = null;
	let accumulated = 0;
	let flushed = false;
	return {
		start() {
			if (!flushed && visibleSince === null) visibleSince = now();
		},
		pause() {
			if (visibleSince !== null) {
				accumulated += now() - visibleSince;
				visibleSince = null;
			}
		},
		/** Returns total visible ms exactly once; null on any later call. */
		flush(): number | null {
			if (flushed) return null;
			this.pause();
			flushed = true;
			return Math.max(0, Math.round(accumulated));
		},
	};
}

// pagehide can outlive the page — keepalive lets the flush survive tab close.
let keepaliveTransport: Transport | null = null;
function createKeepaliveTransport(): Transport {
	if (keepaliveTransport) return keepaliveTransport;
	keepaliveTransport = createConnectTransport({
		baseUrl: `${base}/api/v2`,
		fetch: (input, init) =>
			fetch(input, { ...init, credentials: "include", keepalive: true }),
	});
	return keepaliveTransport;
}

/**
 * useTrailOutcome is the single owner of the trail.act_outcome emit (the
 * PM-2026-045 emit-ownership lesson — no other component may emit it). Call it
 * from the article page during component init. It measures visible dwell and
 * flushes exactly once on leave (navigation, destroy, or pagehide).
 *
 * Passing a null branchKey (no ?trail_proposal= gate) is the organic-visit
 * case: no listeners, no timer, no emit — measurement code never runs.
 */
export function useTrailOutcome(
	branchKey: string | null,
	itemKey: string | null,
): void {
	if (!browser || !branchKey || !itemKey) return;

	const tracker = createDwellTracker();
	if (document.visibilityState === "visible") tracker.start();

	const onVisibility = () => {
		if (document.visibilityState === "visible") tracker.start();
		else tracker.pause();
	};
	document.addEventListener("visibilitychange", onVisibility);

	const flush = (keepalive: boolean) => {
		const dwell = tracker.flush();
		if (dwell === null) return;
		const transport = keepalive
			? createKeepaliveTransport()
			: createClientTransport();
		void emitTrailOutcome(transport, branchKey, itemKey, BigInt(dwell)).catch(
			() => {
				// Best-effort: a lost outcome resolves as no_engagement upstream
				// (absence is a derived state, not an error).
			},
		);
	};
	const onPageHide = () => flush(true);
	window.addEventListener("pagehide", onPageHide);

	beforeNavigate(() => flush(false));
	onDestroy(() => {
		flush(false);
		document.removeEventListener("visibilitychange", onVisibility);
		window.removeEventListener("pagehide", onPageHide);
	});
}

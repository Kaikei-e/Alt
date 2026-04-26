/**
 * makeFirstFrameSkipper — drop the first non-silent stream frame.
 *
 * Background (2026-04-26 production regression): the Knowledge Loop page
 * is rendered SSR-side with the projector's current snapshot already
 * inlined into `data.loop`. The browser then opens the
 * `streamKnowledgeLoopUpdates` subscription in `onMount`. The server
 * always re-emits the current state as the *first* non-silent frame so
 * a fresh subscriber catches up — but the SSR snapshot already owns that
 * state, so kicking `invalidate("loop:data")` in response to that frame
 * is pure churn: it re-runs the load function, replaces `data.loop` with
 * a freshly-allocated object, and SvelteKit re-renders the keyed
 * `{#each}` over the new reference. Any click that was in flight on a
 * foreground article tile during that re-render is dropped.
 *
 * Skipping the first non-silent frame keeps subsequent updates flowing
 * normally while preventing the redundant initial invalidate. Heartbeat
 * frames and `revised` frames are filtered upstream by the page's
 * `onFrame` predicate; this helper only sees the frames that *would*
 * trigger a refresh.
 *
 * The helper is pure (no Svelte runes) so it can be exercised without a
 * component context. Each call returns an independent instance with its
 * own first-frame latch — pages that consume multiple streams should
 * wrap each one separately.
 *
 * `reset()` rearms the latch. The page calls it on stream reconnect
 * (`stream_expired` envelope) because the server replays state on the
 * new connection, so the next first non-silent frame is again redundant
 * with the snapshot the page's load function will refetch.
 */
export interface FirstFrameSkipper {
	(): void;
	reset(): void;
}

export function makeFirstFrameSkipper(forward: () => void): FirstFrameSkipper {
	let armed = true;

	const skipper = (() => {
		if (armed) {
			armed = false;
			return;
		}
		forward();
	}) as FirstFrameSkipper;

	skipper.reset = () => {
		armed = true;
	};

	return skipper;
}

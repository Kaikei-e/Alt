/**
 * Trigger SvelteKit's `updated.check()` immediately on BFCache restore
 * and tab-visibility transitions to "visible", so a stale tab that the
 * user has just returned to learns about a new build before its next
 * client-side navigation requests an evicted `_app/immutable/*` chunk.
 *
 * `version.pollInterval` (ADR-000898, 5min) already nudges long-running
 * tabs, but it cannot react inside the few hundred ms between a BFCache
 * restore and the user's first tap. This installer closes that race.
 *
 * Called from `+layout.svelte` inside a `$effect` so cleanup runs on
 * destroy. Splitting the listener wiring into a plain function makes it
 * unit-testable without spinning up a Svelte component harness.
 */

export interface BfcacheVersionCheckOptions {
	window: Window;
	document: Document;
	check: () => void | Promise<unknown>;
}

export function installBfcacheVersionCheck(
	opts: BfcacheVersionCheckOptions,
): () => void {
	const win = opts.window;
	const doc = opts.document;

	const onPageShow = (event: Event) => {
		const persisted = (event as PageTransitionEvent).persisted;
		if (persisted) {
			void opts.check();
		}
	};

	const onVisibility = () => {
		if (doc.visibilityState === "visible") {
			void opts.check();
		}
	};

	win.addEventListener("pageshow", onPageShow);
	doc.addEventListener("visibilitychange", onVisibility);

	return () => {
		win.removeEventListener("pageshow", onPageShow);
		doc.removeEventListener("visibilitychange", onVisibility);
	};
}

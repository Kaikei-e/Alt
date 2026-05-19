/**
 * Inline-bootstrap recovery for stale `_app/immutable/*` 404s.
 *
 * The same logic is also embedded verbatim inside `src/app.html` so it
 * runs before any SvelteKit runtime chunk has loaded. That early window
 * is the only line of defense when the very entry chunk
 * (`_app/immutable/entry/app.<HASH>.js`) itself 404s after a deploy —
 * `hooks.client.ts` cannot help there because it lives inside the entry
 * chunk that failed to load.
 *
 * `installChunkBootstrap` exists as the testable form. A drift test in
 * `safari-error-bootstrap.test.ts` checks that `BOOTSTRAP_SCRIPT_BODY`
 * is present verbatim inside `app.html`.
 */

const RELOAD_ATTEMPT_KEY = "alt:chunk-reload-attempts";
const RELOAD_ATTEMPT_LIMIT = 3;
const IMMUTABLE_PREFIX = "/_app/immutable/";

export interface ChunkBootstrapOptions {
	window: Window;
	reload?: () => void;
	scheduleReload?: (cb: () => void) => unknown;
}

export function installChunkBootstrap(opts: ChunkBootstrapOptions): () => void {
	const win = opts.window;
	const doReload = opts.reload ?? (() => win.location.reload());
	const schedule =
		opts.scheduleReload ?? ((cb: () => void) => win.setTimeout(cb, 0));
	let fired = false;

	function tryReload(): void {
		if (fired) return;
		fired = true;
		try {
			const storage = win.sessionStorage;
			const prior = Number(storage.getItem(RELOAD_ATTEMPT_KEY) ?? "0");
			if (Number.isFinite(prior) && prior >= RELOAD_ATTEMPT_LIMIT) {
				return;
			}
			storage.setItem(RELOAD_ATTEMPT_KEY, String(prior + 1));
		} catch {
			// Best-effort: a failing sessionStorage (private mode etc.) must
			// not deadlock the recovery — fall through to reload.
		}
		schedule(doReload);
	}

	function isImmutableChunk(target: EventTarget | null): boolean {
		if (!target) return false;
		// Resource load errors (img, script, link, …) come through with the
		// failing element as event.target. We only fire on hashed module
		// scripts under /_app/immutable/.
		const src =
			(target as { src?: string; href?: string }).src ??
			(target as { src?: string; href?: string }).href ??
			"";
		if (typeof src !== "string") return false;
		return src.indexOf(IMMUTABLE_PREFIX) !== -1;
	}

	const onError = (ev: Event) => {
		if (isImmutableChunk(ev.target)) tryReload();
	};
	const onPreloadError = () => tryReload();

	win.addEventListener("error", onError, true);
	win.addEventListener("vite:preloadError", onPreloadError as EventListener);

	return () => {
		win.removeEventListener("error", onError, true);
		win.removeEventListener(
			"vite:preloadError",
			onPreloadError as EventListener,
		);
	};
}

// Canonical source of the inline <script> tag in app.html. The drift
// test compares this against the inline copy with whitespace normalized,
// so biome / prettier reformatting on either side stays compatible while
// any semantic change forces both copies to update in lockstep.
export const BOOTSTRAP_SCRIPT_BODY = `(() => {
	if (typeof window === "undefined") return;
	const K = "alt:chunk-reload-attempts";
	const L = 3;
	let fired = false;
	const go = () => {
		if (fired) return;
		fired = true;
		try {
			const n = Number(window.sessionStorage.getItem(K) || "0");
			if (n >= L) return;
			window.sessionStorage.setItem(K, String(n + 1));
		} catch (e) {}
		window.setTimeout(() => { window.location.reload(); }, 0);
	};
	const isChunk = (t) => {
		if (!t) return false;
		const s = t.src || t.href || "";
		return typeof s === "string" && s.indexOf("/_app/immutable/") !== -1;
	};
	window.addEventListener("error", (e) => { if (isChunk(e.target)) go(); }, true);
	window.addEventListener("vite:preloadError", () => { go(); });
})();`;

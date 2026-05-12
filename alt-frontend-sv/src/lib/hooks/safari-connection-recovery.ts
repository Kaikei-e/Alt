/**
 * Safari Connection Recovery Hook
 *
 * iOS Safari aggressively suspends or discards backgrounded tabs and drops their
 * network connections to save power/memory. When the tab returns it may serve
 * stale content, fail in-flight fetches with NSURLErrorDomain -1004 ("could not
 * connect to server"), or — over HTTP/2 — fail the document reload outright with
 * "サーバに接続できなかったため、ページを開けません。".
 *
 * This hook listens for every signal Safari actually emits on return-from-
 * background — visibilitychange, pageshow (incl. bfcache), the Page Lifecycle
 * `freeze`/`resume` pair, and `online` — and fires `onRecoveryNeeded` so the app
 * can invalidate stale caches and refetch.
 *
 * The hidden timestamp is mirrored into sessionStorage so a long background
 * period is still observable after a bfcache restore or a Safari-initiated
 * document reload, where the in-memory `hiddenAt` would otherwise be lost.
 *
 * Safari-specific behavior:
 * - NSURLErrorDomain -1004: "Could not connect to server" after background
 * - dropped connections surface to fetch() as `TypeError: Load failed`
 * - WebSocket connections silently dropped when the tab loses focus
 * - bfcache restore: `pageshow` with `persisted === true`, all connections stale
 */

export type RecoveryReason = "visibility" | "bfcache" | "online" | "resume";

export interface RecoveryInfo {
	reason: RecoveryReason;
	hiddenDurationMs?: number;
}

type MinimalStorage = Pick<Storage, "getItem" | "setItem" | "removeItem">;

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
	/** Injected for tests. Defaults to globalThis.sessionStorage (if available). */
	storage?: MinimalStorage;
}

export interface SafariConnectionRecoveryHandle {
	dispose(): void;
}

const HIDDEN_AT_KEY = "alt:safari-recovery:hidden-at";

function resolveSessionStorage(): MinimalStorage | undefined {
	try {
		return globalThis.sessionStorage;
	} catch {
		// Safari throws on sessionStorage access in some private-browsing modes.
		return undefined;
	}
}

export function createSafariConnectionRecovery(
	opts: SafariConnectionRecoveryOptions,
): SafariConnectionRecoveryHandle {
	const doc = opts.document ?? globalThis.document;
	const win = opts.window ?? globalThis.window;
	const getNow = opts.getNow ?? Date.now;
	const thresholdMs = opts.thresholdMs;
	const storage = opts.storage ?? resolveSessionStorage();

	let hiddenAt: number | null = null;

	const readStoredHiddenAt = (): number | null => {
		try {
			const raw = storage?.getItem(HIDDEN_AT_KEY);
			if (!raw) return null;
			const n = Number(raw);
			return Number.isFinite(n) ? n : null;
		} catch {
			return null;
		}
	};

	const writeStoredHiddenAt = (value: number | null): void => {
		try {
			if (value === null) storage?.removeItem(HIDDEN_AT_KEY);
			else storage?.setItem(HIDDEN_AT_KEY, String(value));
		} catch {
			// sessionStorage may be unavailable (private mode / SSR) — ignore.
		}
	};

	const markHidden = (): void => {
		hiddenAt = getNow();
		writeStoredHiddenAt(hiddenAt);
	};

	/**
	 * If we are returning from a background period longer than the threshold,
	 * return its duration; otherwise return null. Always clears the hidden marker.
	 */
	const consumeHiddenDuration = (): number | null => {
		const startedAt = hiddenAt ?? readStoredHiddenAt();
		hiddenAt = null;
		writeStoredHiddenAt(null);
		if (startedAt === null) return null;
		const elapsed = getNow() - startedAt;
		return elapsed > thresholdMs ? elapsed : null;
	};

	const onVisibilityChange = (): void => {
		if (doc.visibilityState === "hidden") {
			markHidden();
			return;
		}
		const elapsed = consumeHiddenDuration();
		if (elapsed !== null) {
			opts.onRecoveryNeeded({
				reason: "visibility",
				hiddenDurationMs: elapsed,
			});
		}
	};

	const onPageShow = (e: Event): void => {
		const persisted = (e as { persisted?: boolean }).persisted === true;
		if (persisted) {
			// bfcache restore — connections are definitely stale.
			hiddenAt = null;
			writeStoredHiddenAt(null);
			opts.onRecoveryNeeded({ reason: "bfcache", hiddenDurationMs: undefined });
			return;
		}
		// A non-persisted pageshow fires on every fresh load *and* when Safari
		// reloads a tab it had discarded. Only the latter — detectable because
		// sessionStorage shows we were backgrounded past the threshold — counts.
		const elapsed = consumeHiddenDuration();
		if (elapsed !== null) {
			opts.onRecoveryNeeded({ reason: "resume", hiddenDurationMs: elapsed });
		}
	};

	const onFreeze = (): void => {
		markHidden();
	};

	const onResume = (): void => {
		const elapsed = consumeHiddenDuration();
		if (elapsed !== null) {
			opts.onRecoveryNeeded({ reason: "resume", hiddenDurationMs: elapsed });
		}
	};

	const onOnline = (): void => {
		opts.onRecoveryNeeded({ reason: "online", hiddenDurationMs: undefined });
	};

	doc.addEventListener("visibilitychange", onVisibilityChange);
	doc.addEventListener("pageshow", onPageShow);
	doc.addEventListener("freeze", onFreeze);
	doc.addEventListener("resume", onResume);
	win?.addEventListener("online", onOnline);

	return {
		dispose() {
			doc.removeEventListener("visibilitychange", onVisibilityChange);
			doc.removeEventListener("pageshow", onPageShow);
			doc.removeEventListener("freeze", onFreeze);
			doc.removeEventListener("resume", onResume);
			win?.removeEventListener("online", onOnline);
		},
	};
}

/**
 * True if `error` looks like a browser network-layer failure rather than an
 * application/HTTP error or a deliberate AbortController abort. Safari surfaces
 * a dropped connection as `TypeError: Load failed`; Chromium as
 * `TypeError: Failed to fetch`; Firefox as `TypeError: NetworkError ...`.
 */
export function isNetworkFailureError(error: unknown): boolean {
	if (!(error instanceof Error)) return false;
	if (error.name === "AbortError") return false;
	if (error.name !== "TypeError") return false;
	return /load failed|failed to fetch|network ?error|networkerror/i.test(
		error.message ?? "",
	);
}

const RELOAD_GUARD_KEY = "alt:safari-recovery:last-reload-at";

export interface GuardedReloadOptions {
	/** Injected for tests. Defaults to globalThis. */
	window?: Pick<Window, "location"> & { sessionStorage?: Storage };
	/** Injected for tests. Defaults to globalThis.sessionStorage (if available). */
	storage?: Pick<Storage, "getItem" | "setItem">;
	/** Injected for tests. Defaults to Date.now. */
	getNow?: () => number;
	/** Minimum gap between automatic reloads, ms. Default 60s. */
	cooldownMs?: number;
}

/**
 * Reload the document at most once per cooldown window. Returns true if a reload
 * was actually triggered. Use as an escape hatch when, shortly after a recovery
 * event, fetches keep failing with network errors — i.e. Safari is holding a
 * dead connection that only a fresh navigation will clear. The cooldown
 * (persisted in sessionStorage) prevents reload loops if the failure is
 * server-side rather than a stale connection.
 */
export function performGuardedReload(opts: GuardedReloadOptions = {}): boolean {
	const win =
		opts.window ?? (globalThis as unknown as GuardedReloadOptions["window"]);
	if (!win?.location || typeof win.location.reload !== "function") return false;
	const getNow = opts.getNow ?? Date.now;
	const cooldownMs = opts.cooldownMs ?? 60_000;
	const storage = opts.storage ?? win.sessionStorage ?? resolveSessionStorage();

	let lastReloadAt = 0;
	try {
		const raw = storage?.getItem(RELOAD_GUARD_KEY);
		if (raw) lastReloadAt = Number(raw) || 0;
	} catch {
		// ignore
	}

	const now = getNow();
	if (now - lastReloadAt < cooldownMs) return false;

	try {
		storage?.setItem(RELOAD_GUARD_KEY, String(now));
	} catch {
		// ignore
	}
	win.location.reload();
	return true;
}

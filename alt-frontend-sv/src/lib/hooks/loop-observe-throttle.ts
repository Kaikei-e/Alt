/**
 * 60-second per-entry_key throttle for `KnowledgeLoopObserved` emission
 * (ADR-000831 §8.2, invariant 31). Mirrors the backend rate limit window
 * defined in canonical contract §8.4 — `(user_id, lens_mode_id, entry_key)`
 * per 60s. Keeping the FE window the same shape avoids a tight loop where
 * the page-load tick fires once → backend rejects 429 → FE re-tries on the
 * next IntersectionObserver tick → ad infinitum.
 *
 * Optionally persists the last-emitted timestamps to a Storage (typically
 * `localStorage`). Persistence aligns the FE window with the backend even
 * across page reloads — without it, every reload within the 60s backend
 * window produces a guaranteed 429 per visible entry.
 *
 * Pure factory — Storage is injected so the hook stays unit-testable.
 */

export interface ObserveThrottleStorage {
	getItem(key: string): string | null;
	setItem(key: string, value: string): void;
	removeItem(key: string): void;
}

const STORAGE_KEY = "alt:loop:observe-throttle:v1";

export interface MakeObserveThrottleOptions {
	storage?: ObserveThrottleStorage | null;
}

export function makeObserveThrottle(
	minMs: number,
	opts: MakeObserveThrottleOptions = {},
) {
	const storage = opts.storage ?? null;
	const last = loadFromStorage(storage);

	function persist() {
		if (!storage) return;
		try {
			storage.setItem(STORAGE_KEY, serialize(last));
		} catch {
			// Storage may be unavailable (private browsing, quota). Failures here
			// degrade gracefully to in-memory behavior — a reload will refire.
		}
	}

	return {
		shouldEmit(entryKey: string, nowMs: number): boolean {
			const prev = last.get(entryKey);
			if (prev === undefined || nowMs - prev >= minMs) {
				last.set(entryKey, nowMs);
				persist();
				return true;
			}
			return false;
		},
		reset(entryKey: string) {
			last.delete(entryKey);
			persist();
		},
	};
}

function loadFromStorage(storage: ObserveThrottleStorage | null): Map<string, number> {
	const map = new Map<string, number>();
	if (!storage) return map;
	let raw: string | null = null;
	try {
		raw = storage.getItem(STORAGE_KEY);
	} catch {
		return map;
	}
	if (!raw) return map;
	try {
		const parsed = JSON.parse(raw) as unknown;
		if (parsed && typeof parsed === "object") {
			for (const [k, v] of Object.entries(parsed as Record<string, unknown>)) {
				if (typeof v === "number" && Number.isFinite(v)) {
					map.set(k, v);
				}
			}
		}
	} catch {
		// Corrupt blob — drop it.
	}
	return map;
}

function serialize(last: Map<string, number>): string {
	const obj: Record<string, number> = {};
	for (const [k, v] of last.entries()) obj[k] = v;
	return JSON.stringify(obj);
}

/**
 * 60-second per-(entry_key, bootSessionId) throttle for
 * `KnowledgeLoopObserved` emission (ADR-000831 §8.2, invariant 31).
 *
 * Kept as a pure factory so it can be unit-tested without Svelte runes.
 */
export function makeObserveThrottle(minMs: number) {
	const last = new Map<string, number>();

	return {
		shouldEmit(entryKey: string, nowMs: number): boolean {
			const prev = last.get(entryKey);
			if (prev === undefined || nowMs - prev >= minMs) {
				last.set(entryKey, nowMs);
				return true;
			}
			return false;
		},
		reset(entryKey: string) {
			last.delete(entryKey);
		},
	};
}

import { describe, expect, it } from "vitest";
import {
	MAX_RESOLVE_ATTEMPTS,
	OG_RETRY_BASE_MS,
	OG_RETRY_CEILING_MS,
	ogImageRetryDelayMs,
} from "./ogImageRetry";

describe("ogImageRetryDelayMs", () => {
	it("spreads the wait across the whole window rather than pinning it", () => {
		// Full jitter, not exponential-plus-noise: every client that failed in
		// the same second must not come back in the same second.
		expect(ogImageRetryDelayMs(0, null, () => 0)).toBe(0);
		expect(ogImageRetryDelayMs(0, null, () => 1)).toBe(OG_RETRY_BASE_MS);
		expect(ogImageRetryDelayMs(0, null, () => 0.5)).toBe(OG_RETRY_BASE_MS / 2);
	});

	it("doubles the window with each attempt", () => {
		expect(ogImageRetryDelayMs(1, null, () => 1)).toBe(OG_RETRY_BASE_MS * 2);
		expect(ogImageRetryDelayMs(2, null, () => 1)).toBe(OG_RETRY_BASE_MS * 4);
	});

	it("clamps the window so a thumbnail never shimmers past the ceiling", () => {
		expect(ogImageRetryDelayMs(20, null, () => 1)).toBe(OG_RETRY_CEILING_MS);
	});

	it("treats the server's Retry-After as a floor, never a suggestion", () => {
		// Re-asking inside a gate the server just closed spends a slot on a
		// request it has already told us it will refuse.
		expect(ogImageRetryDelayMs(0, 8_000, () => 0)).toBe(8_000);
		// ...but the jittered window still wins when it is the longer wait.
		expect(ogImageRetryDelayMs(2, 1_000, () => 1)).toBe(OG_RETRY_BASE_MS * 4);
	});

	it("clamps an absurd Retry-After to the same ceiling", () => {
		expect(ogImageRetryDelayMs(0, 3_600_000, () => 0)).toBe(
			OG_RETRY_CEILING_MS,
		);
	});

	it("keeps the ladder short enough to be worth waiting through", () => {
		// The bound is the point: an unreachable backend must land the card on
		// the fallback rather than shimmering for the rest of the session.
		expect(MAX_RESOLVE_ATTEMPTS).toBeLessThanOrEqual(3);
		const worstCase = Array.from(
			{ length: MAX_RESOLVE_ATTEMPTS - 1 },
			(_, attempt) => ogImageRetryDelayMs(attempt, null, () => 1),
		).reduce((a, b) => a + b, 0);
		expect(worstCase).toBeLessThanOrEqual(5_000);
	});
});

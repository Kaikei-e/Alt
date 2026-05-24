import { describe, expect, it } from "vitest";

import { resolveReviewReason } from "./review-reason-map";

describe("resolveReviewReason", () => {
	it("returns undefined when entry is not in the Review bucket", () => {
		expect(
			resolveReviewReason({ dismissState: "active", surfaceBucket: "now" }),
		).toBeUndefined();
		expect(
			resolveReviewReason({
				dismissState: "completed",
				surfaceBucket: "continue",
			}),
		).toBeUndefined();
		expect(
			resolveReviewReason({
				dismissState: "active",
				surfaceBucket: "changed",
			}),
		).toBeUndefined();
	});

	it("active in Review reads as Stale evidence", () => {
		const got = resolveReviewReason({
			dismissState: "active",
			surfaceBucket: "review",
		});
		expect(got).toBeDefined();
		expect(got?.reason).toBe("stale_evidence");
		expect(got?.label).toBe("Stale evidence");
	});

	it("dismissed in Review reads as Previously dismissed (v1 fallback)", () => {
		const got = resolveReviewReason({
			dismissState: "dismissed",
			surfaceBucket: "review",
		});
		expect(got?.reason).toBe("previously_dismissed");
		expect(got?.label).toBe("Previously dismissed");
	});

	it("completed in Review reads as Reviewed (mark_reviewed survivor)", () => {
		const got = resolveReviewReason({
			dismissState: "completed",
			surfaceBucket: "review",
		});
		expect(got?.reason).toBe("reviewed");
		expect(got?.label).toBe("Reviewed");
	});

	it("deferred in Review reads as Deferred (future-proof)", () => {
		const got = resolveReviewReason({
			dismissState: "deferred",
			surfaceBucket: "review",
		});
		expect(got?.reason).toBe("deferred");
		expect(got?.label).toBe("Deferred");
	});

	it("stale evidence and previously dismissed never share copy", () => {
		const stale = resolveReviewReason({
			dismissState: "active",
			surfaceBucket: "review",
		});
		const dismissed = resolveReviewReason({
			dismissState: "dismissed",
			surfaceBucket: "review",
		});
		expect(stale?.label).not.toBe(dismissed?.label);
		expect(stale?.kicker).not.toBe(dismissed?.kicker);
	});

	it("is deterministic across repeated invocations (reproject parity)", () => {
		for (const dismissState of [
			"active",
			"dismissed",
			"completed",
			"deferred",
		] as const) {
			const first = resolveReviewReason({
				dismissState,
				surfaceBucket: "review",
			});
			for (let i = 0; i < 5; i++) {
				expect(
					resolveReviewReason({ dismissState, surfaceBucket: "review" }),
				).toEqual(first);
			}
		}
	});

	// ADR-000908 §Δ3: an internalized entry should never appear in any
	// bucket — including Review. The helper returns undefined so the caller
	// skips the reason chip entirely.
	it("internalized never produces a review reason (filtered upstream)", () => {
		for (const bucket of ["now", "continue", "changed", "review"] as const) {
			expect(
				resolveReviewReason({
					dismissState: "internalized",
					surfaceBucket: bucket,
				}),
			).toBeUndefined();
		}
	});
});

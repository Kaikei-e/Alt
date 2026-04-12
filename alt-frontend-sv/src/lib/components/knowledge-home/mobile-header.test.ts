import { describe, expect, it } from "vitest";
import { getQualityDotClass, getQualityLabel } from "./mobile-header";

describe("getQualityDotClass (Alt-Paper ink tones)", () => {
	it("returns alt-success for full", () => {
		expect(getQualityDotClass("full")).toContain("alt-success");
	});

	it("returns alt-warning with pulse for degraded", () => {
		const cls = getQualityDotClass("degraded");
		expect(cls).toContain("alt-warning");
		expect(cls).toContain("animate-pulse");
	});

	it("returns alt-error for fallback", () => {
		expect(getQualityDotClass("fallback")).toContain("alt-error");
	});

	it("does not reference legacy badge-* tokens", () => {
		for (const q of ["full", "degraded", "fallback"] as const) {
			const cls = getQualityDotClass(q);
			expect(cls).not.toMatch(/badge-(green|amber|orange)/);
		}
	});
});

describe("getQualityLabel", () => {
	it("returns accessible label for each quality", () => {
		expect(getQualityLabel("full")).toBe("Service status: full");
		expect(getQualityLabel("degraded")).toBe("Service status: degraded");
		expect(getQualityLabel("fallback")).toBe("Service status: fallback");
	});
});

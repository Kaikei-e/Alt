import { describe, expect, it } from "vitest";
import { getQualityDotClass, getQualityLabel } from "./mobile-header";

describe("getQualityDotClass", () => {
	it("returns green for full", () => {
		expect(getQualityDotClass("full")).toContain("green");
	});

	it("returns amber with pulse for degraded", () => {
		const cls = getQualityDotClass("degraded");
		expect(cls).toContain("amber");
		expect(cls).toContain("animate-pulse");
	});

	it("returns orange for fallback", () => {
		expect(getQualityDotClass("fallback")).toContain("orange");
	});
});

describe("getQualityLabel", () => {
	it("returns accessible label for each quality", () => {
		expect(getQualityLabel("full")).toBe("Service status: full");
		expect(getQualityLabel("degraded")).toBe("Service status: degraded");
		expect(getQualityLabel("fallback")).toBe("Service status: fallback");
	});
});

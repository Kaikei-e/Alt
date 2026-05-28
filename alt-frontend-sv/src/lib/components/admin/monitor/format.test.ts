import { describe, expect, it } from "vitest";

import { burnSeverity, formatValue, stateBadge } from "./format";

describe("formatValue", () => {
	it("returns em dash for null and non-finite values", () => {
		expect(formatValue(null, "seconds")).toBe("—");
		expect(formatValue(Number.NaN, "seconds")).toBe("—");
		expect(formatValue(Number.POSITIVE_INFINITY, "ratio")).toBe("—");
	});

	it("renders seconds as ms below 1s and as s above", () => {
		expect(formatValue(0.123, "seconds")).toBe("123 ms");
		expect(formatValue(1.5, "seconds")).toBe("1.50 s");
	});

	it("renders ratio as percent with two decimals", () => {
		expect(formatValue(0.0142, "ratio")).toBe("1.42%");
	});

	it("renders bool as up/down", () => {
		expect(formatValue(1, "bool")).toBe("up");
		expect(formatValue(0, "bool")).toBe("down");
	});

	it("renders bytes in binary prefix", () => {
		expect(formatValue(1024, "bytes")).toBe("1.00 KiB");
		expect(formatValue(5 * 1024 * 1024, "bytes")).toBe("5.00 MiB");
	});
});

describe("stateBadge", () => {
	it("returns warn glyph for missing values (data is not 'ok' just because we can't see it)", () => {
		expect(stateBadge(null, "seconds")).toEqual({ glyph: "○", text: "warn" });
	});

	it("maps bool to up/down arrows so color is never the sole channel", () => {
		expect(stateBadge(1, "bool")).toEqual({ glyph: "▲", text: "up" });
		expect(stateBadge(0, "bool")).toEqual({ glyph: "▼", text: "down" });
	});

	it("uses the warn predicate to flip the glyph to a solid dot", () => {
		const warn = (v: number) => v > 1.0;
		expect(stateBadge(0.5, "seconds", warn)).toEqual({
			glyph: "▲",
			text: "ok",
		});
		expect(stateBadge(1.5, "seconds", warn)).toEqual({
			glyph: "●",
			text: "warn",
		});
	});
});

describe("burnSeverity (99.9% SLO baseline)", () => {
	it("buckets by SRE workbook thresholds", () => {
		expect(burnSeverity(null)).toBe("ok");
		expect(burnSeverity(0.5)).toBe("ok");
		expect(burnSeverity(1)).toBe("ticket");
		expect(burnSeverity(5.99)).toBe("ticket");
		expect(burnSeverity(6)).toBe("page2");
		expect(burnSeverity(14)).toBe("page2");
		expect(burnSeverity(14.4)).toBe("page1");
		expect(burnSeverity(100)).toBe("page1");
	});
});

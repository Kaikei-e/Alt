import { describe, expect, it } from "vitest";
import { computeDelta, topSeries, type SimpleSeries } from "./util";

function pts(values: number[]): { time: string; value: number }[] {
	return values.map((v, i) => ({
		time: new Date(1_700_000_000_000 + i * 15_000).toISOString(),
		value: v,
	}));
}

describe("computeDelta", () => {
	it("returns null when fewer than 4 points", () => {
		expect(computeDelta(pts([]))).toBeNull();
		expect(computeDelta(pts([1, 2, 3]))).toBeNull();
	});

	it("compares the trailing half to the leading half", () => {
		const d = computeDelta(pts([10, 10, 10, 20, 20, 20]));
		expect(d).not.toBeNull();
		// leading avg = 10, trailing avg = 20, abs delta = 10, pct = 100%
		expect(d!.absolute).toBeCloseTo(10, 3);
		expect(d!.percent).toBeCloseTo(100, 1);
		expect(d!.direction).toBe("up");
	});

	it("returns direction=down and a negative percent when trend decreases", () => {
		const d = computeDelta(pts([40, 40, 40, 20, 20, 20]));
		expect(d!.absolute).toBeCloseTo(-20, 3);
		expect(d!.percent).toBeCloseTo(-50, 1);
		expect(d!.direction).toBe("down");
	});

	it("returns direction=flat when the change is below the flat threshold", () => {
		const d = computeDelta(pts([10, 10, 10, 10.01, 10, 10.02]));
		expect(d!.direction).toBe("flat");
	});

	it("handles zero leading average without blowing up", () => {
		const d = computeDelta(pts([0, 0, 0, 5, 5, 5]));
		expect(d!.absolute).toBeCloseTo(5, 3);
		// percent undefined when baseline is zero; function should report Infinity-safe value
		expect(Number.isFinite(d!.percent)).toBe(true);
		expect(d!.direction).toBe("up");
	});
});

describe("topSeries", () => {
	const mk = (label: string, values: number[]): SimpleSeries => ({
		labels: { job: label },
		points: pts(values),
	});

	it("returns all series and overflow=0 when count <= limit", () => {
		const r = topSeries([mk("a", [1]), mk("b", [2])], "job", 3);
		expect(r.head.map((s) => s.labelValue)).toEqual(["b", "a"]);
		expect(r.overflow).toBe(0);
	});

	it("sorts by lead value desc and caps at limit", () => {
		const series = [
			mk("alpha", [1]),
			mk("beta", [5]),
			mk("gamma", [3]),
			mk("delta", [7]),
			mk("epsilon", [2]),
		];
		const r = topSeries(series, "job", 3);
		expect(r.head.map((s) => s.labelValue)).toEqual(["delta", "beta", "gamma"]);
		expect(r.overflow).toBe(2);
	});

	it("falls back to a composite label when the preferred label is missing", () => {
		const s: SimpleSeries = {
			labels: { pool: "read", shard: "2" },
			points: pts([1]),
		};
		const r = topSeries([s], "job", 3);
		expect(r.head[0].labelValue).toContain("pool=read");
	});

	it("returns empty head + 0 overflow for empty input", () => {
		const r = topSeries([], "job", 3);
		expect(r.head).toEqual([]);
		expect(r.overflow).toBe(0);
	});
});

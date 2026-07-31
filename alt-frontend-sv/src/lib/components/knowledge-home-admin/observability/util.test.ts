import { describe, expect, it } from "vitest";
import {
	computeDelta,
	serviceHealthRows,
	topSeries,
	type SimpleSeries,
} from "./util";

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
		expect(r.head[0]!.labelValue).toContain("pool=read");
	});

	it("returns empty head + 0 overflow for empty input", () => {
		const r = topSeries([], "job", 3);
		expect(r.head).toEqual([]);
		expect(r.overflow).toBe(0);
	});
});

describe("serviceHealthRows", () => {
	const mk = (job: string, value: number): SimpleSeries => ({
		labels: { job },
		points: pts([value]),
	});

	it("derives a row for every job in the series, including new ones", () => {
		// plecto-proxy is the edge proxy. It was scraped but absent from the
		// component's hardcoded list, so a fully-down edge rendered as no row
		// at all — indistinguishable from a healthy stack.
		const rows = serviceHealthRows([
			mk("alt-backend", 1),
			mk("plecto-proxy", 0),
		]);
		expect(rows.map((r) => r.job)).toEqual(["alt-backend", "plecto-proxy"]);
		expect(rows.find((r) => r.job === "plecto-proxy")!.up).toBe(0);
	});

	it("sorts rows by job name", () => {
		const rows = serviceHealthRows([
			mk("rag-orchestrator", 1),
			mk("cadvisor", 1),
			mk("mq-hub", 1),
		]);
		expect(rows.map((r) => r.job)).toEqual([
			"cadvisor",
			"mq-hub",
			"rag-orchestrator",
		]);
	});

	it("collapses several series of one job to the worst target", () => {
		// Defense in depth: the backend aggregates pki-agent's eight sidecars
		// with min by (job), but if that ever regresses the table must not let
		// a healthy sibling overwrite a dead one.
		const rows = serviceHealthRows([
			mk("pki-agent", 1),
			mk("pki-agent", 0),
			mk("pki-agent", 1),
		]);
		expect(rows).toHaveLength(1);
		expect(rows[0]!.up).toBe(0);
	});

	it("ignores series with no points and returns [] for empty input", () => {
		expect(serviceHealthRows([])).toEqual([]);
		expect(serviceHealthRows([{ labels: { job: "x" }, points: [] }])).toEqual(
			[],
		);
	});

	it("labels a series with no job label rather than dropping it", () => {
		const rows = serviceHealthRows([{ labels: {}, points: pts([0]) }]);
		expect(rows).toEqual([{ job: "(unknown)", up: 0 }]);
	});
});

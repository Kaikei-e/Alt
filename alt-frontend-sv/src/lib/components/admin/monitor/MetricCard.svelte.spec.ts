import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-svelte";

import {
	MetricResultSchema,
	PointSchema,
	SeriesKind,
	SeriesSchema,
} from "#lib/gen/alt/admin_monitor/v1/admin_monitor_pb.js";
import MetricCard from "./MetricCard.svelte";
import { THRESHOLDS } from "./model";

function makeMetric(opts: {
	unit: string;
	values: number[];
	labels?: Record<string, string>;
	degraded?: boolean;
	reason?: string;
}) {
	const points = opts.values.map((v, i) =>
		create(PointSchema, {
			time: new Date(1_700_000_000_000 + i * 30_000).toISOString(),
			value: v,
		}),
	);
	const series = create(SeriesSchema, {
		labels: opts.labels ?? { job: "alt-backend" },
		points,
	});
	return create(MetricResultSchema, {
		key: "test",
		kind: SeriesKind.RANGE,
		unit: opts.unit,
		series: [series],
		degraded: !!opts.degraded,
		reason: opts.reason ?? "",
	});
}

describe("MetricCard", () => {
	it("renders the lead value in the configured unit and threshold display", () => {
		const metric = makeMetric({ unit: "seconds", values: [0.1, 0.2, 0.34] });
		const screen = render(MetricCard, {
			props: {
				label: "Edge p95 latency",
				metric,
				threshold: THRESHOLDS.edgeLatencyP95,
			},
		});
		const num = screen.container.querySelector(".num");
		expect(num?.textContent).toContain("340 ms");
		const threshold = screen.container.querySelector(".threshold-text");
		expect(threshold?.textContent).toBe("warn ≥ 1.00 s · crit ≥ 2.50 s");
	});

	it("encodes state with text + glyph, not color alone (WCAG 1.4.1)", () => {
		const metric = makeMetric({ unit: "seconds", values: [0.5, 0.6, 1.5] });
		const screen = render(MetricCard, {
			props: {
				label: "Edge p95 latency",
				metric,
				threshold: THRESHOLDS.edgeLatencyP95,
			},
		});
		const card = screen.container.querySelector('[data-testid="metric-card"]');
		expect(card?.getAttribute("data-state")).toBe("warn");
		const glyph = card?.querySelector(".glyph");
		expect(glyph?.textContent?.trim()).toBe("▲");
		const stateText = card?.querySelector(".state-text");
		expect(stateText?.textContent?.trim()).toBe("WARN");
	});

	it("shows degraded reason when the metric source is degraded", () => {
		const metric = makeMetric({
			unit: "ratio",
			values: [0.01],
			degraded: true,
			reason: "prometheus query timeout",
		});
		const screen = render(MetricCard, {
			props: { label: "Edge 5xx", metric },
		});
		const reason = screen.container.querySelector(".reason");
		expect(reason?.textContent).toContain("prometheus query timeout");
		const card = screen.container.querySelector('[data-testid="metric-card"]');
		expect(card?.classList.contains("degraded")).toBe(true);
	});

	it("falls back to NO DATA badge and emptyText for missing series (never 'warn' on missing data)", () => {
		const screen = render(MetricCard, {
			props: {
				label: "Edge p95",
				metric: undefined,
				emptyText: "no traffic",
			},
		});
		const stateText = screen.container.querySelector(".state-text");
		expect(stateText?.textContent?.trim()).toBe("NO DATA");
		const num = screen.container.querySelector(".num");
		expect(num?.textContent?.trim()).toBe("no traffic");
	});

	it("uses only the first matching series without concatenating multiple series", () => {
		const series1 = create(SeriesSchema, {
			labels: { job: "alt-backend" },
			points: [
				create(PointSchema, { time: "2026-05-29T00:00:00Z", value: 4 }),
				create(PointSchema, { time: "2026-05-29T00:00:30Z", value: 6 }),
			],
		});
		const series2 = create(SeriesSchema, {
			labels: { job: "mq-hub" },
			points: [
				create(PointSchema, { time: "2026-05-29T00:00:00Z", value: 99 }),
			],
		});
		const metric = create(MetricResultSchema, {
			key: "edge_rps",
			kind: SeriesKind.RANGE,
			unit: "req/s",
			series: [series1, series2],
		});
		const screen = render(MetricCard, {
			props: { label: "Traffic", metric },
		});
		const num = screen.container.querySelector(".num");
		expect(num?.textContent).toContain("6.00");
		const svg = screen.container.querySelector("svg[aria-label]");
		expect(svg?.getAttribute("aria-label")).toContain("latest 6.00");
		expect(svg?.getAttribute("aria-label")).toContain("max 6.00");
	});
});

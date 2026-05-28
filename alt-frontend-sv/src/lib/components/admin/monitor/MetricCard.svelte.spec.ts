import { render } from "vitest-browser-svelte";
import { describe, expect, it } from "vitest";
import { create } from "@bufbuild/protobuf";

import {
	MetricResultSchema,
	SeriesSchema,
	PointSchema,
	SeriesKind,
} from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import MetricCard from "./MetricCard.svelte";

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
	it("renders the lead value in the configured unit", () => {
		const metric = makeMetric({ unit: "seconds", values: [0.1, 0.2, 0.34] });
		const screen = render(MetricCard, {
			props: { label: "HTTP p95", metric },
		});
		const num = screen.container.querySelector(".num");
		expect(num?.textContent).toContain("340 ms");
	});

	it("encodes state with text + glyph, not color alone (WCAG 1.4.1)", () => {
		const metric = makeMetric({ unit: "seconds", values: [0.5, 0.6, 1.5] });
		const screen = render(MetricCard, {
			props: {
				label: "HTTP p95",
				metric,
				warn: (v: number) => v > 1.0,
			},
		});
		const card = screen.container.querySelector('[data-testid="metric-card"]');
		expect(card?.getAttribute("data-state")).toBe("warn");
		const glyph = card?.querySelector(".glyph");
		expect(glyph?.textContent?.trim()).toBe("●");
		const stateText = card?.querySelector(".state-text");
		expect(stateText?.textContent?.trim()).toBe("warn");
	});

	it("shows degraded reason when the metric source is degraded", () => {
		const metric = makeMetric({
			unit: "ratio",
			values: [0.01],
			degraded: true,
			reason: "prometheus query timeout",
		});
		const screen = render(MetricCard, {
			props: { label: "HTTP 5xx", metric },
		});
		const reason = screen.container.querySelector(".reason");
		expect(reason?.textContent).toContain("prometheus query timeout");
		const card = screen.container.querySelector('[data-testid="metric-card"]');
		expect(card?.classList.contains("degraded")).toBe(true);
	});

	it("falls back to em-dash badge for missing series (no 'ok' on missing data)", () => {
		const screen = render(MetricCard, {
			props: { label: "HTTP p95", metric: undefined },
		});
		const stateText = screen.container.querySelector(".state-text");
		expect(stateText?.textContent?.trim()).toBe("warn");
		const dim = screen.container.querySelector(".dim");
		expect(dim?.textContent).toContain("no series");
	});

	it("aggregates across series when aggregate=sum", () => {
		const series1 = create(SeriesSchema, {
			labels: { job: "alt-backend" },
			points: [create(PointSchema, { time: "2026-05-29T00:00:00Z", value: 4 })],
		});
		const series2 = create(SeriesSchema, {
			labels: { job: "mq-hub" },
			points: [create(PointSchema, { time: "2026-05-29T00:00:00Z", value: 7 })],
		});
		const metric = create(MetricResultSchema, {
			key: "http_rps",
			kind: SeriesKind.RANGE,
			unit: "req/s",
			series: [series1, series2],
		});
		const screen = render(MetricCard, {
			props: { label: "Traffic", metric, aggregate: "sum" },
		});
		const num = screen.container.querySelector(".num");
		expect(num?.textContent).toContain("11.00");
	});
});

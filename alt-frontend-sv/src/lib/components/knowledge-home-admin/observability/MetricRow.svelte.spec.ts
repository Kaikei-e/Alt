import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it } from "vitest";
import { create } from "@bufbuild/protobuf";

import {
	MetricResultSchema,
	SeriesSchema,
	PointSchema,
	SeriesKind,
} from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import MetricRow from "./MetricRow.svelte";

function makeMetric(opts: {
	key: string;
	unit: string;
	values: number[];
	labels?: Record<string, string>;
	degraded?: boolean;
	reason?: string;
	grafanaUrl?: string;
}) {
	const points = opts.values.map((v, i) =>
		create(PointSchema, {
			time: new Date(1_700_000_000_000 + i * 15_000).toISOString(),
			value: v,
		}),
	);
	const series = create(SeriesSchema, {
		labels: opts.labels ?? { job: "alt-backend" },
		points,
	});
	return create(MetricResultSchema, {
		key: opts.key,
		kind: SeriesKind.RANGE,
		unit: opts.unit,
		series: [series],
		degraded: !!opts.degraded,
		reason: opts.reason ?? "",
		grafanaUrl: opts.grafanaUrl ?? "",
	});
}

describe("MetricRow", () => {
	it("formats ratio units as percentages", async () => {
		const metric = makeMetric({
			key: "http_error_ratio",
			unit: "ratio",
			values: [0.002, 0.005, 0.015],
		});
		const screen = render(MetricRow as never, {
			props: { metric, label: "HTTP 5xx ratio" },
		});
		// Primary value rendering — digest may repeat the value, so scope to .value-num.
		const el = screen.container.querySelector(".value-num");
		expect(el?.textContent).toContain("1.50%");
	});

	it("marks the row as warn when threshold triggers", async () => {
		const metric = makeMetric({
			key: "http_error_ratio",
			unit: "ratio",
			values: [0.02, 0.04],
		});
		const screen = render(MetricRow as never, {
			props: {
				metric,
				label: "HTTP 5xx ratio",
				threshold: { warn: (v: number) => v > 0.01 },
			},
		});
		const row = screen.container.querySelector(".row");
		expect(row?.classList.contains("warn")).toBe(true);
	});

	it("renders a delta glyph with direction when enough points exist", async () => {
		const metric = makeMetric({
			key: "http_latency_p95",
			unit: "seconds",
			values: [0.1, 0.1, 0.1, 0.2, 0.2, 0.2],
		});
		render(MetricRow as never, {
			props: { metric, label: "HTTP p95 latency" },
		});
		// Glyph is aria-hidden; look for the percentage text, which is visible.
		await expect.element(page.getByText(/\+100\.0%/)).toBeInTheDocument();
	});

	it("renders degraded reason as subtext", async () => {
		const metric = makeMetric({
			key: "availability_services",
			unit: "bool",
			values: [1],
			degraded: true,
			reason: "prometheus_timeout",
		});
		render(MetricRow as never, { props: { metric, label: "Availability" } });
		await expect.element(page.getByText(/prometheus_timeout/)).toBeInTheDocument();
	});

	it("does not render the Grafana link when no URL is set", async () => {
		const metric = makeMetric({
			key: "http_rps",
			unit: "req/s",
			values: [1, 2, 3, 4],
			grafanaUrl: "",
		});
		const screen = render(MetricRow as never, {
			props: { metric, label: "Traffic" },
		});
		expect(screen.container.querySelector(".grafana")).toBeNull();
	});

	it("shows em-dash when the metric has no series", async () => {
		render(MetricRow as never, {
			props: { metric: undefined, label: "Traffic" },
		});
		await expect.element(page.getByLabelText(/Traffic —/)).toBeInTheDocument();
	});
});

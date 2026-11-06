import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-svelte";

import {
	MetricResultSchema,
	PointSchema,
	SeriesKind,
	SeriesSchema,
} from "#lib/gen/alt/admin_monitor/v1/admin_monitor_pb.js";
import { buildServiceRows, indexMetrics } from "./model";
import ServiceREDTable from "./ServiceREDTable.svelte";

function makeSeries(
	labels: Record<string, string>,
	points: Array<{ time: string; value: number }>,
) {
	return create(SeriesSchema, {
		labels,
		points: points.map((p) => create(PointSchema, p)),
	});
}

function makeMetric(
	key: string,
	series: ReturnType<typeof makeSeries>[],
	unit = "",
) {
	return create(MetricResultSchema, {
		key,
		kind: SeriesKind.RANGE,
		unit,
		series,
		degraded: false,
		reason: "",
	});
}

describe("ServiceREDTable", () => {
	const t0 = "2026-11-06T12:00:00Z";
	const t1 = "2026-11-06T12:00:30Z";

	const metrics = [
		makeMetric(
			"availability_services",
			[
				makeSeries({ job: "alt-backend" }, [{ time: t1, value: 1 }]),
				makeSeries({ job: "news-creator" }, [{ time: t1, value: 1 }]),
				makeSeries({ job: "news-creator-ops" }, [{ time: t1, value: 0 }]),
				makeSeries({ job: "recap-evaluator" }, [{ time: t1, value: 1 }]),
			],
			"bool",
		),
		makeMetric(
			"http_rps",
			[
				makeSeries({ job: "alt-backend", signal: "http_server" }, [
					{ time: t0, value: 12 },
					{ time: t1, value: 15 },
				]),
				makeSeries({ job: "news-creator", signal: "http_server" }, [
					{ time: t0, value: 1.0 },
					{ time: t1, value: 1.5 },
				]),
			],
			"req/s",
		),
		makeMetric(
			"http_latency_p95",
			[
				makeSeries({ job: "alt-backend", signal: "http_server" }, [
					{ time: t0, value: 0.08 },
					{ time: t1, value: 0.09 },
				]),
				makeSeries({ job: "news-creator", signal: "http_server" }, [
					{ time: t0, value: 0.4 },
					{ time: t1, value: 0.45 },
				]),
			],
			"seconds",
		),
	];

	it("renders group headers, merged targets, accessible state reasons, and uninstrumented RED cells", () => {
		const metricMap = indexMetrics(metrics);
		const screen = render(ServiceREDTable, {
			props: { metrics: metricMap, rows: buildServiceRows(metricMap) },
		});

		const groupHeaders = Array.from(
			screen.container.querySelectorAll(".group-row th"),
		).map((el) => el.textContent?.trim());
		expect(groupHeaders).toEqual(["API & BFF", "Workers"]);

		const newsRow = screen.container.querySelector(
			'tr[data-service="news-creator"]',
		);
		expect(newsRow?.textContent).toContain("1/2");
		expect(newsRow?.textContent).toContain("CRIT");
		expect(newsRow?.textContent).toContain("no data");
		expect(newsRow?.textContent).not.toContain("0.00%");

		const toggleBtn = newsRow?.querySelector("button[aria-expanded]");
		expect(toggleBtn).not.toBeNull();
		expect(toggleBtn?.getAttribute("aria-expanded")).toBe("false");
		const controlsId = toggleBtn?.getAttribute("aria-controls");
		expect(controlsId).toBe("drill-news-creator");
		expect(screen.container.querySelector(`#${controlsId}`)).not.toBeNull();

		const stateCell = newsRow?.querySelector(".state-cell");
		expect(stateCell?.getAttribute("aria-label")).toBe(
			"CRIT: 1/2 targets down",
		);

		const recapRow = screen.container.querySelector(
			'tr[data-service="recap-evaluator"]',
		);
		expect(recapRow?.textContent).toContain("no request metrics");
	});

	it("renders 'no data' instead of 'no request metrics' when http_* keys are absent", () => {
		const onlyUp = [
			makeMetric(
				"availability_services",
				[makeSeries({ job: "recap-evaluator" }, [{ time: t1, value: 1 }])],
				"bool",
			),
		];
		const metricMap = indexMetrics(onlyUp);
		const screen = render(ServiceREDTable, {
			props: { metrics: metricMap, rows: buildServiceRows(metricMap) },
		});
		const recapRow = screen.container.querySelector(
			'tr[data-service="recap-evaluator"]',
		);
		expect(recapRow?.textContent).toContain("no data");
		expect(recapRow?.textContent).not.toContain("no request metrics");
	});

	it("distinguishes pre-snapshot waiting text from post-snapshot missing target availability", () => {
		const preSnapshot = render(ServiceREDTable, {
			props: { metrics: indexMetrics([]), rows: [] },
		});
		expect(preSnapshot.container.textContent).toContain(
			"Waiting for first stream tick",
		);

		const postSnapshotMap = indexMetrics([
			makeMetric("availability_services", [], "bool"),
		]);
		const postSnapshot = render(ServiceREDTable, {
			props: {
				metrics: postSnapshotMap,
				rows: buildServiceRows(postSnapshotMap),
			},
		});
		expect(postSnapshot.container.textContent).toContain(
			"NO DATA — target availability unavailable",
		);
		expect(postSnapshot.container.textContent).not.toContain(
			"Waiting for first stream tick",
		);
	});
});

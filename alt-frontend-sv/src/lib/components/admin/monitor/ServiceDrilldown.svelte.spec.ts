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
import ServiceDrilldown from "./ServiceDrilldown.svelte";

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

describe("ServiceDrilldown", () => {
	const t1 = "2026-11-06T12:00:30Z";

	it("shows 'Targets up · RED unknown' when row is ok but redMetricsAvailable is false", () => {
		const onlyUpMap = indexMetrics([
			makeMetric(
				"availability_services",
				[makeSeries({ job: "recap-evaluator" }, [{ time: t1, value: 1 }])],
				"bool",
			),
		]);
		const rows = buildServiceRows(onlyUpMap);
		const row = rows[0];
		expect(row).toBeDefined();
		if (!row) throw new Error("expected service row");
		const screen = render(ServiceDrilldown, {
			props: { row, metrics: onlyUpMap },
		});
		expect(screen.container.textContent).toContain("Targets up · RED unknown");
		expect(screen.container.textContent).toContain(
			"NO DATA — request metrics unavailable.",
		);
		expect(screen.container.textContent).not.toContain(
			"All thresholds passing",
		);
	});
});

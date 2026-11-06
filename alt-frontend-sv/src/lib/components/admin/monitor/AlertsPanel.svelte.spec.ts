import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-svelte";

import {
	MetricResultSchema,
	PointSchema,
	SeriesKind,
	SeriesSchema,
} from "#lib/gen/alt/admin_monitor/v1/admin_monitor_pb.js";
import AlertsPanel from "./AlertsPanel.svelte";
import { alertsAvailability, buildAlertRows, indexMetrics } from "./model";

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

describe("AlertsPanel", () => {
	const t1 = "2026-11-06T12:00:30Z";

	it("formats activeSince unix seconds against millisecond now and renders NO DATA when availability is false", () => {
		const alertMap = indexMetrics([
			makeMetric(
				"alerts_firing",
				[
					makeSeries(
						{
							alertname: "TargetDown",
							severity: "critical",
							job: "mq-hub",
							alertstate: "firing",
						},
						[{ time: t1, value: 1 }],
					),
				],
				"bool",
			),
			makeMetric("alerts_pending", [], "bool"),
			makeMetric(
				"alerts_active_since",
				[
					makeSeries(
						{
							alertname: "TargetDown",
							severity: "critical",
							job: "mq-hub",
						},
						[{ time: t1, value: 1_700_000_000 }],
					),
				],
				"unix_seconds",
			),
		]);
		const alertScreen = render(AlertsPanel, {
			props: {
				rows: buildAlertRows(alertMap),
				now: 1_700_000_120_000,
				availability: alertsAvailability(alertMap),
			},
		});
		expect(alertScreen.container.textContent).toContain("2m ago");

		const emptyMap = indexMetrics([]);
		const unavailableScreen = render(AlertsPanel, {
			props: {
				rows: [],
				now: 1_700_000_120_000,
				availability: alertsAvailability(emptyMap),
			},
		});
		expect(unavailableScreen.container.textContent).toContain(
			"NO DATA — alert state unavailable",
		);
	});
});

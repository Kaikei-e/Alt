import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-svelte";

import {
	MetricResultSchema,
	PointSchema,
	SeriesKind,
	SeriesSchema,
} from "#lib/gen/alt/admin_monitor/v1/admin_monitor_pb.js";
import HostResources from "./HostResources.svelte";
import { buildHostResources, indexMetrics } from "./model";

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

describe("HostResources", () => {
	it("shows NO DATA reason when host_filesystem_utilization is missing and empty text only when healthy", () => {
		const missingMap = indexMetrics([]);
		const missingScreen = render(HostResources, {
			props: { metrics: missingMap, resources: buildHostResources(missingMap) },
		});
		expect(missingScreen.container.textContent).toContain(
			"NO DATA — filesystem utilization unavailable (missing host_filesystem_utilization)",
		);
		expect(missingScreen.container.textContent).not.toContain(
			"No filesystem utilization series reported.",
		);

		const healthyEmptyMap = indexMetrics([
			makeMetric("host_filesystem_utilization", [], "ratio"),
		]);
		const healthyEmptyScreen = render(HostResources, {
			props: {
				metrics: healthyEmptyMap,
				resources: buildHostResources(healthyEmptyMap),
			},
		});
		expect(healthyEmptyScreen.container.textContent).toContain(
			"No filesystem utilization series reported.",
		);
	});
});

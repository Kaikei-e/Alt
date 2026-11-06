import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-svelte";

import {
	MetricResultSchema,
	PointSchema,
	SeriesKind,
	SeriesSchema,
} from "#lib/gen/alt/admin_monitor/v1/admin_monitor_pb.js";
import { buildPipelines, indexMetrics } from "./model";
import PipelinesPanel from "./PipelinesPanel.svelte";

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

describe("PipelinesPanel", () => {
	it("shows NO DATA reasons when publish rate or backlog metrics are missing and empty text only when healthy", () => {
		const missingScreen = render(PipelinesPanel, {
			props: { pipelines: buildPipelines(indexMetrics([])) },
		});
		expect(missingScreen.container.textContent).toContain(
			"NO DATA — publish rate unavailable (missing mqhub_publish_rate)",
		);
		expect(missingScreen.container.textContent).not.toContain(
			"No active publish streams.",
		);
		expect(missingScreen.container.textContent).toContain(
			"NO DATA — queue backlog unavailable (missing backlog_oldest_age)",
		);
		expect(missingScreen.container.textContent).not.toContain(
			"No queue backlogs reported.",
		);

		const healthyEmptyScreen = render(PipelinesPanel, {
			props: {
				pipelines: buildPipelines(
					indexMetrics([
						makeMetric("mqhub_publish_rate", [], "msg/s"),
						makeMetric("backlog_oldest_age", [], "seconds"),
					]),
				),
			},
		});
		expect(healthyEmptyScreen.container.textContent).toContain(
			"No active publish streams.",
		);
		expect(healthyEmptyScreen.container.textContent).toContain(
			"No queue backlogs reported.",
		);
	});
});

import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { render } from "vitest-browser-svelte";

import {
	MetricResultSchema,
	PointSchema,
	SeriesKind,
	SeriesSchema,
} from "#lib/gen/alt/admin_monitor/v1/admin_monitor_pb.js";
import CertificatesPanel from "./CertificatesPanel.svelte";
import { buildCertRows, certsAvailability, indexMetrics } from "./model";

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

describe("CertificatesPanel", () => {
	it("shows NO DATA reason when certificate metrics are missing and empty text only when healthy", () => {
		const missingMap = indexMetrics([]);
		const missingScreen = render(CertificatesPanel, {
			props: {
				rows: buildCertRows(missingMap),
				availability: certsAvailability(missingMap),
			},
		});
		expect(missingScreen.container.textContent).toContain(
			"NO DATA — certificate enrollment unavailable (missing cert_remaining, cert_healthy)",
		);
		expect(missingScreen.container.textContent).not.toContain(
			"No certificate enrollment metrics reported.",
		);

		const healthyEmptyMap = indexMetrics([
			makeMetric("cert_remaining", [], "seconds"),
			makeMetric("cert_healthy", [], "bool"),
		]);
		const healthyEmptyScreen = render(CertificatesPanel, {
			props: {
				rows: buildCertRows(healthyEmptyMap),
				availability: certsAvailability(healthyEmptyMap),
			},
		});
		expect(healthyEmptyScreen.container.textContent).toContain(
			"No certificate enrollment metrics reported.",
		);
	});
});

import { render } from "vitest-browser-svelte";
import { describe, expect, it } from "vitest";
import { create } from "@bufbuild/protobuf";

import {
	MetricResultSchema,
	SeriesKind,
} from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import MonitorErrorBanner from "./MonitorErrorBanner.svelte";

function makeMetric(degraded: boolean, reason = "") {
	return create(MetricResultSchema, {
		key: "k",
		kind: SeriesKind.INSTANT,
		unit: "bool",
		degraded,
		reason,
		series: [],
	});
}

describe("MonitorErrorBanner", () => {
	it("stays hidden while the stream is live and no metric is degraded", () => {
		const screen = render(MonitorErrorBanner, {
			props: { metrics: [makeMetric(false)], streamState: "live" },
		});
		expect(
			screen.container.querySelector('[data-testid="monitor-error-banner"]'),
		).toBeNull();
	});

	it("appears when any metric reports degraded=true", () => {
		const screen = render(MonitorErrorBanner, {
			props: {
				metrics: [makeMetric(false), makeMetric(true, "prometheus timeout")],
				streamState: "live",
			},
		});
		const banner = screen.container.querySelector(
			'[data-testid="monitor-error-banner"]',
		);
		expect(banner).not.toBeNull();
		expect(banner?.textContent).toContain("prometheus timeout");
	});

	it("appears while the stream is connecting (no first frame yet)", () => {
		const screen = render(MonitorErrorBanner, {
			props: { metrics: [], streamState: "connecting" },
		});
		const banner = screen.container.querySelector(
			'[data-testid="monitor-error-banner"]',
		);
		expect(banner?.textContent).toContain("Connecting");
	});

	it("encodes the state with text 'degraded' + glyph ●, not color alone", () => {
		const screen = render(MonitorErrorBanner, {
			props: {
				metrics: [makeMetric(true, "prometheus unreachable")],
				streamState: "live",
			},
		});
		const head = screen.container.querySelector(".head");
		expect(head?.textContent).toContain("●");
		expect(head?.textContent?.toLowerCase()).toContain("degraded");
	});
});

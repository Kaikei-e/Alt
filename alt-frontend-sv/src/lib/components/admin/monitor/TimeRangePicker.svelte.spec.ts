import { render } from "vitest-browser-svelte";
import { describe, expect, it } from "vitest";

import {
	RangeWindow,
	Step,
} from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";
import TimeRangePicker from "./TimeRangePicker.svelte";

describe("TimeRangePicker", () => {
	it("renders both window and step segmented controls", () => {
		const screen = render(TimeRangePicker, {
			props: { window: RangeWindow.RANGE_WINDOW_1H, step: Step.STEP_30S },
		});
		const labels = Array.from(
			screen.container.querySelectorAll(".seg span"),
		).map((el) => el.textContent?.trim());
		expect(labels).toEqual(
			expect.arrayContaining(["5m", "15m", "1h", "6h", "24h"]),
		);
		expect(labels).toEqual(expect.arrayContaining(["15s", "30s", "1m", "5m"]));
	});

	it("marks the currently bound window/step as active", () => {
		const screen = render(TimeRangePicker, {
			props: { window: RangeWindow.RANGE_WINDOW_24H, step: Step.STEP_5M },
		});
		const active = Array.from(
			screen.container.querySelectorAll(".seg.active span"),
		).map((el) => el.textContent?.trim());
		expect(active).toEqual(expect.arrayContaining(["24h", "5m"]));
	});

	it("disables step values that would exceed the 720-point server guard", () => {
		// 24h window + 15s step → 5760 points → must be disabled.
		const screen = render(TimeRangePicker, {
			props: { window: RangeWindow.RANGE_WINDOW_24H, step: Step.STEP_1M },
		});
		const disabled = screen.container.querySelector(
			'input[name="monitor-step"][value="1"]', // Step.STEP_15S = 1
		) as HTMLInputElement | null;
		expect(disabled?.disabled).toBe(true);
	});

	it("does not over-disable steps that fit within 720 points (1h × 15s = 240)", () => {
		const screen = render(TimeRangePicker, {
			props: { window: RangeWindow.RANGE_WINDOW_1H, step: Step.STEP_30S },
		});
		const input = screen.container.querySelector(
			'input[name="monitor-step"][value="1"]', // Step.STEP_15S
		) as HTMLInputElement | null;
		expect(input?.disabled).toBe(false);
	});
});

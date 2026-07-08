import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";

import {
	RangeWindow,
	Step,
} from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";

const watchSpy = vi.fn();
const catalogSpy = vi.fn();

vi.mock("$lib/connect/transport-client", () => ({
	createClientTransport: () => ({}),
}));

vi.mock("$lib/connect/admin_monitor", () => ({
	createAdminMonitorClient: () => ({
		catalog: catalogSpy.mockResolvedValue({ entries: [] }),
		watch: (req: unknown) => {
			watchSpy(req);
			return (async function* () {
				yield { time: "2026-05-29T00:00:00Z", metrics: [] };
			})();
		},
	}),
}));

import { useConnectAdminMetrics } from "./useConnectAdminMetrics.svelte";

describe("useConnectAdminMetrics — picker-driven config", () => {
	beforeEach(() => {
		watchSpy.mockClear();
		catalogSpy.mockClear();
	});

	afterEach(() => {
		vi.useRealTimers();
	});

	it("forwards picker-provided keys, window, and step to the stream request", async () => {
		const hook = useConnectAdminMetrics({
			keys: ["http_latency_p50", "availability_burn_1h"],
			window: RangeWindow.RANGE_WINDOW_24H,
			step: Step.STEP_5M,
		});
		hook.start();

		await vi.waitFor(() => expect(watchSpy).toHaveBeenCalled());

		const req = watchSpy.mock.calls[0]![0];
		expect(req.keys).toEqual(["http_latency_p50", "availability_burn_1h"]);
		expect(req.window).toBe(RangeWindow.RANGE_WINDOW_24H);
		expect(req.step).toBe(Step.STEP_5M);

		hook.stop();
	});

	it("falls back to default keys when no opts are passed (existing tab call-site contract)", async () => {
		const hook = useConnectAdminMetrics();
		hook.start();

		await vi.waitFor(() => expect(watchSpy).toHaveBeenCalled());

		const req = watchSpy.mock.calls[0]![0];
		expect(req.keys.length).toBeGreaterThan(0);
		expect(req.keys).toContain("http_latency_p95");
		expect(req.window).toBe(RangeWindow.RANGE_WINDOW_UNSPECIFIED);
		expect(req.step).toBe(Step.STEP_UNSPECIFIED);

		hook.stop();
	});
});

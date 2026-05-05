import { describe, it, expect, vi } from "vitest";
import { createAndAutostart } from "./acolyteAutostart";
import { AcolyteRpcError } from "./acolyte";

describe("createAndAutostart", () => {
	const inputs = {
		title: "Iran Outlook 2026",
		reportType: "weekly_briefing",
		scope: { topic: "Iran" },
	};

	it("calls createReport then startReportRun then navigates with ?run=", async () => {
		const order: string[] = [];
		const createReport = vi.fn(async () => {
			order.push("createReport");
			return { reportId: "rpt-1" };
		});
		const startReportRun = vi.fn(async () => {
			order.push("startReportRun");
			return { runId: "run-1" };
		});
		const goto = vi.fn(async (_url: string) => {
			order.push("goto");
		});

		const result = await createAndAutostart(
			{ createReport, startReportRun, goto },
			inputs.title,
			inputs.reportType,
			inputs.scope,
		);

		expect(result).toEqual({ ok: true, reportId: "rpt-1", runId: "run-1" });
		expect(order).toEqual(["createReport", "startReportRun", "goto"]);
		expect(createReport).toHaveBeenCalledWith(
			"Iran Outlook 2026",
			"weekly_briefing",
			{ topic: "Iran" },
		);
		expect(startReportRun).toHaveBeenCalledWith("rpt-1");
		expect(goto).toHaveBeenCalledWith("/acolyte/reports/rpt-1?run=run-1");
	});

	it("navigates with ?autostart_failed=1 when startReportRun fails after createReport succeeds", async () => {
		const createReport = vi.fn(async () => ({ reportId: "rpt-2" }));
		const startReportRun = vi.fn(async () => {
			throw new AcolyteRpcError("failed_precondition", "already running");
		});
		const goto = vi.fn(async () => {});

		const result = await createAndAutostart(
			{ createReport, startReportRun, goto },
			inputs.title,
			inputs.reportType,
			inputs.scope,
		);

		expect(result.ok).toBe(false);
		expect(result.reportId).toBe("rpt-2");
		expect(result.runId).toBeUndefined();
		expect(goto).toHaveBeenCalledWith(
			"/acolyte/reports/rpt-2?autostart_failed=1",
		);
	});

	it("does NOT call startReportRun or goto when createReport fails", async () => {
		const createReport = vi.fn(async () => {
			throw new AcolyteRpcError("internal", "boom");
		});
		const startReportRun = vi.fn(async () => ({ runId: "run-3" }));
		const goto = vi.fn(async () => {});

		const result = await createAndAutostart(
			{ createReport, startReportRun, goto },
			inputs.title,
			inputs.reportType,
			inputs.scope,
		);

		expect(result.ok).toBe(false);
		expect(result.reportId).toBeUndefined();
		expect(result.error).toBe("boom");
		expect(startReportRun).not.toHaveBeenCalled();
		expect(goto).not.toHaveBeenCalled();
	});

	it("propagates an Error.message even when not an AcolyteRpcError", async () => {
		const createReport = vi.fn(async () => {
			throw new Error("network down");
		});
		const startReportRun = vi.fn();
		const goto = vi.fn();

		const result = await createAndAutostart(
			{ createReport, startReportRun, goto },
			inputs.title,
			inputs.reportType,
			inputs.scope,
		);

		expect(result.ok).toBe(false);
		expect(result.error).toBe("network down");
	});

	it("falls back to a generic message for non-Error throw values", async () => {
		const createReport = vi.fn(async () => {
			throw "boom";
		});
		const result = await createAndAutostart(
			{
				createReport,
				startReportRun: vi.fn(),
				goto: vi.fn(),
			},
			inputs.title,
			inputs.reportType,
			inputs.scope,
		);
		expect(result.ok).toBe(false);
		expect(result.error).toBe("Failed to create report");
	});
});

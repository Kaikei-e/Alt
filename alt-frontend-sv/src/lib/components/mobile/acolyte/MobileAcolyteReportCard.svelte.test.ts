import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";
import MobileAcolyteReportCard from "./MobileAcolyteReportCard.svelte";
import { MOCK_REPORT_SUMMARIES } from "./acolyte-fixtures";

describe("MobileAcolyteReportCard", () => {
	const succeededReport = MOCK_REPORT_SUMMARIES[0]; // succeeded
	const runningReport = MOCK_REPORT_SUMMARIES[1]; // running
	const failedReport = MOCK_REPORT_SUMMARIES[2]; // failed
	const draftReport = MOCK_REPORT_SUMMARIES[3]; // draft

	it("renders report title", async () => {
		render(MobileAcolyteReportCard as never, {
			props: {
				report: succeededReport,
				onClick: vi.fn(),
			},
		});

		await expect
			.element(page.getByText("AI Semiconductor Supply Chain Analysis"))
			.toBeInTheDocument();
	});

	it("renders report type in uppercase", async () => {
		render(MobileAcolyteReportCard as never, {
			props: {
				report: succeededReport,
				onClick: vi.fn(),
			},
		});

		await expect.element(page.getByText("weekly briefing")).toBeInTheDocument();
	});

	it("renders version badge", async () => {
		render(MobileAcolyteReportCard as never, {
			props: {
				report: succeededReport,
				onClick: vi.fn(),
			},
		});

		await expect.element(page.getByText("v2")).toBeInTheDocument();
	});

	it("renders status label for succeeded", async () => {
		render(MobileAcolyteReportCard as never, {
			props: {
				report: succeededReport,
				onClick: vi.fn(),
			},
		});

		await expect.element(page.getByText("Complete")).toBeInTheDocument();
	});

	it("renders status label for running", async () => {
		render(MobileAcolyteReportCard as never, {
			props: {
				report: runningReport,
				onClick: vi.fn(),
			},
		});

		await expect.element(page.getByText("Running")).toBeInTheDocument();
	});

	it("renders status label for failed", async () => {
		render(MobileAcolyteReportCard as never, {
			props: {
				report: failedReport,
				onClick: vi.fn(),
			},
		});

		await expect.element(page.getByText("Failed")).toBeInTheDocument();
	});

	it("renders status label for draft", async () => {
		render(MobileAcolyteReportCard as never, {
			props: {
				report: draftReport,
				onClick: vi.fn(),
			},
		});

		await expect.element(page.getByText("Draft")).toBeInTheDocument();
	});

	it("has data-testid attribute", async () => {
		render(MobileAcolyteReportCard as never, {
			props: {
				report: succeededReport,
				onClick: vi.fn(),
			},
		});

		await expect
			.element(page.getByTestId("report-card-rpt-001"))
			.toBeInTheDocument();
	});

	it("calls onClick when card is tapped", async () => {
		const onClick = vi.fn();
		render(MobileAcolyteReportCard as never, {
			props: {
				report: succeededReport,
				onStartRun: vi.fn(),
				onClick,
			},
		});

		await page.getByTestId("report-card-rpt-001").click();
		expect(onClick).toHaveBeenCalledWith("rpt-001");
	});

	it("does not render a run button", async () => {
		render(MobileAcolyteReportCard as never, {
			props: {
				report: succeededReport,
				onClick: vi.fn(),
			},
		});

		const btn = page.getByRole("button", { name: /generate/i });
		await expect.element(btn).not.toBeInTheDocument();
	});

	it("applies status stripe color class", async () => {
		render(MobileAcolyteReportCard as never, {
			props: {
				report: succeededReport,
				onClick: vi.fn(),
			},
		});

		const stripe = page.getByTestId("status-stripe-rpt-001");
		await expect.element(stripe).toBeInTheDocument();
		await expect.element(stripe).toHaveAttribute("data-status", "succeeded");
	});
});

import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it } from "vitest";
import MobileAcolyteList from "./MobileAcolyteList.svelte";
import { MOCK_REPORT_SUMMARIES } from "./acolyte-fixtures";

describe("MobileAcolyteList", () => {
	it("renders masthead with title Acolyte", async () => {
		render(MobileAcolyteList as never, {
			props: {
				reports: MOCK_REPORT_SUMMARIES,
				loading: false,
				error: null,
			},
		});

		await expect.element(page.getByText("Acolyte")).toBeInTheDocument();
	});

	it("renders report count", async () => {
		render(MobileAcolyteList as never, {
			props: {
				reports: MOCK_REPORT_SUMMARIES,
				loading: false,
				error: null,
			},
		});

		await expect.element(page.getByText(/4 reports/)).toBeInTheDocument();
	});

	it("renders all report cards", async () => {
		render(MobileAcolyteList as never, {
			props: {
				reports: MOCK_REPORT_SUMMARIES,
				loading: false,
				error: null,
			},
		});

		await expect
			.element(page.getByTestId("report-card-rpt-001"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("report-card-rpt-002"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("report-card-rpt-003"))
			.toBeInTheDocument();
		await expect
			.element(page.getByTestId("report-card-rpt-004"))
			.toBeInTheDocument();
	});

	it("renders New Report link", async () => {
		render(MobileAcolyteList as never, {
			props: {
				reports: MOCK_REPORT_SUMMARIES,
				loading: false,
				error: null,
			},
		});

		const link = page.getByRole("link", { name: /new report/i });
		await expect.element(link).toBeInTheDocument();
		await expect.element(link).toHaveAttribute("href", "/acolyte/new");
	});

	it("shows loading state", async () => {
		render(MobileAcolyteList as never, {
			props: {
				reports: [],
				loading: true,
				error: null,
			},
		});

		await expect
			.element(page.getByText(/retrieving reports/i))
			.toBeInTheDocument();
	});

	it("shows empty state when no reports", async () => {
		render(MobileAcolyteList as never, {
			props: {
				reports: [],
				loading: false,
				error: null,
			},
		});

		await expect
			.element(page.getByText(/no reports have been composed/i))
			.toBeInTheDocument();
	});

	it("shows error message", async () => {
		render(MobileAcolyteList as never, {
			props: {
				reports: [],
				loading: false,
				error: "Network failure",
			},
		});

		await expect.element(page.getByText("Network failure")).toBeInTheDocument();
	});

	it("renders singular report count for 1 report", async () => {
		render(MobileAcolyteList as never, {
			props: {
				reports: [MOCK_REPORT_SUMMARIES[0]],
				loading: false,
				error: null,
			},
		});

		await expect.element(page.getByText(/1 report$/)).toBeInTheDocument();
	});
});

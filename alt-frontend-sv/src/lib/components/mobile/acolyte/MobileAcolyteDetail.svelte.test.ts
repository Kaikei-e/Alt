import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";
import MobileAcolyteDetail from "./MobileAcolyteDetail.svelte";
import { MOCK_REPORT, MOCK_SECTIONS, MOCK_VERSIONS } from "./acolyte-fixtures";

describe("MobileAcolyteDetail", () => {
	const baseProps = {
		report: MOCK_REPORT,
		sections: MOCK_SECTIONS,
		versions: MOCK_VERSIONS,
		loading: false,
		error: null,
		generating: false,
		onGenerate: vi.fn(),
		onRerun: vi.fn(),
	};

	it("renders report title", async () => {
		render(MobileAcolyteDetail as never, { props: baseProps });

		await expect
			.element(page.getByText("AI Semiconductor Supply Chain Analysis"))
			.toBeInTheDocument();
	});

	it("renders report type and date", async () => {
		render(MobileAcolyteDetail as never, { props: baseProps });

		await expect
			.element(page.getByText(/weekly briefing/i))
			.toBeInTheDocument();
	});

	it("renders edition badge", async () => {
		render(MobileAcolyteDetail as never, { props: baseProps });

		await expect.element(page.getByText("Edition 2")).toBeInTheDocument();
	});

	it("renders back link to /acolyte", async () => {
		render(MobileAcolyteDetail as never, { props: baseProps });

		const link = page.getByRole("link", { name: /all reports/i });
		await expect.element(link).toHaveAttribute("href", "/acolyte");
	});

	it("renders section tabs", async () => {
		render(MobileAcolyteDetail as never, { props: baseProps });

		await expect.element(page.getByText("overview")).toBeInTheDocument();
		await expect.element(page.getByText("market trends")).toBeInTheDocument();
	});

	it("renders Generate button", async () => {
		render(MobileAcolyteDetail as never, { props: baseProps });

		await expect
			.element(page.getByRole("button", { name: /generate/i }))
			.toBeInTheDocument();
	});

	it("renders History button", async () => {
		render(MobileAcolyteDetail as never, { props: baseProps });

		await expect
			.element(page.getByRole("button", { name: /history/i }))
			.toBeInTheDocument();
	});

	it("shows loading state", async () => {
		render(MobileAcolyteDetail as never, {
			props: { ...baseProps, report: null, loading: true },
		});

		await expect
			.element(page.getByTestId("detail-loading"))
			.toBeInTheDocument();
	});

	it("shows error state", async () => {
		render(MobileAcolyteDetail as never, {
			props: { ...baseProps, report: null, error: "Not found" },
		});

		await expect.element(page.getByText("Not found")).toBeInTheDocument();
	});

	it("shows empty body when no sections", async () => {
		render(MobileAcolyteDetail as never, {
			props: { ...baseProps, sections: [] },
		});

		await expect.element(page.getByText(/no content yet/i)).toBeInTheDocument();
	});

	it("renders Rerun button for active section", async () => {
		render(MobileAcolyteDetail as never, { props: baseProps });

		await expect
			.element(page.getByRole("button", { name: /rerun/i }))
			.toBeInTheDocument();
	});

	it("Generate button is disabled when generating is true", async () => {
		render(MobileAcolyteDetail as never, {
			props: { ...baseProps, generating: true },
		});

		const btn = page.getByRole("button", { name: /generat/i });
		await expect.element(btn).toBeDisabled();
	});

	it("Generate button is enabled when generating is false", async () => {
		render(MobileAcolyteDetail as never, {
			props: { ...baseProps, generating: false },
		});

		const btn = page.getByRole("button", { name: /generate/i });
		await expect.element(btn).not.toBeDisabled();
	});

	it("Generate button shows 'Generating…' text when generating", async () => {
		render(MobileAcolyteDetail as never, {
			props: { ...baseProps, generating: true },
		});

		await expect
			.element(page.getByText("Generating\u2026"))
			.toBeInTheDocument();
	});
});

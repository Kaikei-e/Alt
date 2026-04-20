import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";
import MobileAcolyteDetail from "./MobileAcolyteDetail.svelte";
import { MOCK_REPORT, MOCK_SECTIONS, MOCK_VERSIONS } from "./acolyte-fixtures";
import type { AcolyteSection } from "$lib/connect/acolyte";

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

	describe("citation overflow on narrow viewports", () => {
		const LONG_CITATION_SECTIONS: AcolyteSection[] = [
			{
				sectionKey: "executive_summary",
				currentVersion: 1,
				displayOrder: 1,
				body: "Body text.",
				citationsJson: JSON.stringify([
					{
						claim_id: "executive_summary-accepted-1",
						source_type: "article",
						source_id: "936a9156-a70b-48ee-9efa-a8c6f4d4a538",
						quote:
							'<p><img src="https://imgu.web.nhk/news/u/news/html/20260317/K10015077511_2603170417_0317045358_01_02.jpg" alt="K10015077">',
					},
				]),
			},
		];

		it("wraps long UUID source ids instead of overflowing horizontally", async () => {
			render(MobileAcolyteDetail as never, {
				props: { ...baseProps, sections: LONG_CITATION_SECTIONS },
			});

			const locator = page.getByText(
				"article:936a9156-a70b-48ee-9efa-a8c6f4d4a538",
			);
			await expect.element(locator).toBeInTheDocument();
			const el = locator.element() as HTMLElement;
			const styles = window.getComputedStyle(el);
			const allowsBreak =
				styles.overflowWrap === "anywhere" ||
				styles.wordBreak === "break-all" ||
				styles.wordBreak === "break-word";
			expect(allowsBreak).toBe(true);
		});

		it("wraps long quote strings that contain raw URLs without breaks", async () => {
			render(MobileAcolyteDetail as never, {
				props: { ...baseProps, sections: LONG_CITATION_SECTIONS },
			});

			const locator = page.getByText(/imgu\.web\.nhk/);
			await expect.element(locator).toBeInTheDocument();
			const el = locator.element() as HTMLElement;
			const styles = window.getComputedStyle(el);
			const allowsBreak =
				styles.overflowWrap === "anywhere" ||
				styles.wordBreak === "break-all" ||
				styles.wordBreak === "break-word";
			expect(allowsBreak).toBe(true);
		});

		it("constrains each source list item to the viewport width", async () => {
			render(MobileAcolyteDetail as never, {
				props: { ...baseProps, sections: LONG_CITATION_SECTIONS },
			});

			const locator = page.getByText(
				"article:936a9156-a70b-48ee-9efa-a8c6f4d4a538",
			);
			const span = locator.element() as HTMLElement;
			const li = span.closest("li") as HTMLElement;
			expect(li).not.toBeNull();
			// min-width: 0 is the escape hatch that lets a flex child shrink
			// below its content; without it long tokens blow out the parent.
			expect(window.getComputedStyle(li).minWidth).toBe("0px");
		});
	});
});

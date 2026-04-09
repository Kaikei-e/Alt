import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";
import MobileAcolyteNew from "./MobileAcolyteNew.svelte";

describe("MobileAcolyteNew", () => {
	it("renders form header", async () => {
		render(MobileAcolyteNew as never, {
			props: { onSubmit: vi.fn() },
		});

		await expect
			.element(page.getByText("Compose New Report"))
			.toBeInTheDocument();
	});

	it("renders back link to /acolyte", async () => {
		render(MobileAcolyteNew as never, {
			props: { onSubmit: vi.fn() },
		});

		const link = page.getByRole("link", { name: /all reports/i });
		await expect.element(link).toBeInTheDocument();
		await expect.element(link).toHaveAttribute("href", "/acolyte");
	});

	it("renders title input", async () => {
		render(MobileAcolyteNew as never, {
			props: { onSubmit: vi.fn() },
		});

		const input = page.getByLabelText(/title/i);
		await expect.element(input).toBeInTheDocument();
	});

	it("renders all 4 report type options", async () => {
		render(MobileAcolyteNew as never, {
			props: { onSubmit: vi.fn() },
		});

		await expect.element(page.getByText("Weekly Briefing")).toBeInTheDocument();
		await expect.element(page.getByText("Market Analysis")).toBeInTheDocument();
		await expect
			.element(page.getByText("Technology Review"))
			.toBeInTheDocument();
		await expect.element(page.getByText("Custom Report")).toBeInTheDocument();
	});

	it("renders topic textarea", async () => {
		render(MobileAcolyteNew as never, {
			props: { onSubmit: vi.fn() },
		});

		const textarea = page.getByLabelText(/topic/i);
		await expect.element(textarea).toBeInTheDocument();
	});

	it("submit button is disabled when title is empty", async () => {
		render(MobileAcolyteNew as never, {
			props: { onSubmit: vi.fn() },
		});

		const btn = page.getByRole("button", { name: /create report/i });
		await expect.element(btn).toBeDisabled();
	});

	it("renders cancel link", async () => {
		render(MobileAcolyteNew as never, {
			props: { onSubmit: vi.fn() },
		});

		const link = page.getByRole("link", { name: /cancel/i });
		await expect.element(link).toHaveAttribute("href", "/acolyte");
	});

	it("shows error message when error prop is set", async () => {
		render(MobileAcolyteNew as never, {
			props: { onSubmit: vi.fn(), error: "Title is required" },
		});

		await expect
			.element(page.getByText("Title is required"))
			.toBeInTheDocument();
	});

	it("shows submitting state", async () => {
		render(MobileAcolyteNew as never, {
			props: { onSubmit: vi.fn(), submitting: true },
		});

		await expect.element(page.getByText(/creating/i)).toBeInTheDocument();
	});
});

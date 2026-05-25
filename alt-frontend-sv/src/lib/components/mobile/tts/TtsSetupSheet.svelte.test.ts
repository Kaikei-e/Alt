import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import TtsSetupSheet from "./TtsSetupSheet.svelte";

const SPEED_CHOICES = [0.75, 1.0, 1.25, 1.5, 2.0];

function baseProps(
	overrides: Partial<Parameters<typeof render>[1]["props"]> = {},
) {
	return {
		open: true,
		hasSummary: true,
		hasBody: true,
		speed: 1.0,
		speedChoices: SPEED_CHOICES,
		onClose: vi.fn(),
		onStart: vi.fn(),
		...overrides,
	};
}

describe("TtsSetupSheet", () => {
	it("renders the sheet title when open", async () => {
		render(TtsSetupSheet, { props: baseProps() });
		await expect
			.element(page.getByText(/listen to article/i))
			.toBeInTheDocument();
	});

	it("offers Summary and Body content choices", async () => {
		render(TtsSetupSheet, { props: baseProps() });

		await expect
			.element(page.getByRole("radio", { name: /summary/i }))
			.toBeInTheDocument();
		await expect
			.element(page.getByRole("radio", { name: /article/i }))
			.toBeInTheDocument();
	});

	it("disables Summary when no summary is available", async () => {
		render(TtsSetupSheet, { props: baseProps({ hasSummary: false }) });

		await expect
			.element(page.getByRole("radio", { name: /summary/i }))
			.toBeDisabled();
	});

	it("disables Body when no body content is available", async () => {
		render(TtsSetupSheet, { props: baseProps({ hasBody: false }) });

		await expect
			.element(page.getByRole("radio", { name: /article/i }))
			.toBeDisabled();
	});

	it("renders all speed choices as radio buttons with aria-checked", async () => {
		render(TtsSetupSheet, { props: baseProps({ speed: 1.25 }) });

		const oneX = page.getByRole("radio", { name: /1\.25x/i });
		await expect.element(oneX).toBeInTheDocument();
		await expect.element(oneX).toHaveAttribute("aria-checked", "true");
	});

	it("calls onStart with the chosen source and speed", async () => {
		const onStart = vi.fn();
		render(TtsSetupSheet, { props: baseProps({ onStart, speed: 1.5 }) });

		// User picks Article (body) explicitly.
		await page.getByRole("radio", { name: /article/i }).click();
		await page.getByRole("button", { name: /^start/i }).click();

		expect(onStart).toHaveBeenCalledWith("body", 1.5);
	});

	it("calls onClose when the Close control is activated", async () => {
		const onClose = vi.fn();
		render(TtsSetupSheet, { props: baseProps({ onClose }) });

		await page.getByRole("button", { name: /^close$/i }).click();
		expect(onClose).toHaveBeenCalled();
	});
});

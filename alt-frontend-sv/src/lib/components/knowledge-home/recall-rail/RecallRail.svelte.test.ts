import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import RecallRail from "./RecallRail.svelte";

describe("RecallRail", () => {
	it("shows an empty state when there are no candidates", async () => {
		render(RecallRail as never, {
			props: {
				candidates: [],
				onSnooze: vi.fn(),
				onDismiss: vi.fn(),
				onOpen: vi.fn(),
			},
		});

		await expect
			.element(page.getByText("Nothing to recall right now."))
			.toBeInTheDocument();
	});

	it("shows unavailable message when unavailable prop is true", async () => {
		render(RecallRail as never, {
			props: {
				candidates: [],
				unavailable: true,
				onSnooze: vi.fn(),
				onDismiss: vi.fn(),
				onOpen: vi.fn(),
			},
		});

		await expect
			.element(page.getByText("Recall is temporarily unavailable."))
			.toBeInTheDocument();
	});

	it("shows heading", async () => {
		render(RecallRail as never, {
			props: {
				candidates: [],
				onSnooze: vi.fn(),
				onDismiss: vi.fn(),
				onOpen: vi.fn(),
			},
		});

		await expect
			.element(page.getByRole("heading", { name: "Recall" }))
			.toBeInTheDocument();
	});
});

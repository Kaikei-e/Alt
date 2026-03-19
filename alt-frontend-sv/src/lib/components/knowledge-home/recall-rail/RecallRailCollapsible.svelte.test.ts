import { describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import RecallRailCollapsible from "./RecallRailCollapsible.svelte";

describe("RecallRailCollapsible", () => {
	it("shows unavailable message when unavailable prop is true", async () => {
		render(RecallRailCollapsible as never, {
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
});

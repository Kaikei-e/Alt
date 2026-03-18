import { render } from "vitest-browser-svelte";
import { page } from "vitest/browser";
import { describe, expect, it, vi } from "vitest";
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
});

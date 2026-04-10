import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";
import FeedFilters from "./FeedFilters.svelte";

describe("FeedFilters", () => {
	it("renders Unread Only checkbox label", async () => {
		render(FeedFilters as never, {
			props: { onFilterChange: vi.fn() },
		});

		await expect.element(page.getByText("Unread Only")).toBeInTheDocument();
	});

	it("renders sort dropdown", async () => {
		render(FeedFilters as never, {
			props: { onFilterChange: vi.fn() },
		});

		await expect.element(page.getByText("Date (Newest)")).toBeInTheDocument();
	});
});

import { describe, expect, it, vi } from "vitest";
import { page, userEvent } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import TrailSearch from "./TrailSearch.svelte";

// Wave 9: trail search is the sole rediscovery instrument (D25). Pull-only —
// the component must never call onSearch except on explicit submit.
describe("TrailSearch", () => {
	it("submitting the form calls onSearch with the trimmed query", async () => {
		const onSearch = vi.fn();
		render(TrailSearch, {
			props: { active: false, searching: false, onSearch, onClear: vi.fn() },
		});
		const input = page.getByTestId("trail-search");
		await input.fill("  submarine  ");
		await userEvent.keyboard("{Enter}");
		expect(onSearch).toHaveBeenCalledWith("submarine");
	});

	it("submitting an empty query is a no-op", async () => {
		const onSearch = vi.fn();
		render(TrailSearch, {
			props: { active: false, searching: false, onSearch, onClear: vi.fn() },
		});
		const input = page.getByTestId("trail-search");
		await input.click();
		await userEvent.keyboard("{Enter}");
		expect(onSearch).not.toHaveBeenCalled();
	});

	it("submitting a whitespace-only query is a no-op", async () => {
		const onSearch = vi.fn();
		render(TrailSearch, {
			props: { active: false, searching: false, onSearch, onClear: vi.fn() },
		});
		const input = page.getByTestId("trail-search");
		await input.fill("   ");
		await userEvent.keyboard("{Enter}");
		expect(onSearch).not.toHaveBeenCalled();
	});

	it("does not render the clear affordance while inactive", async () => {
		render(TrailSearch, {
			props: {
				active: false,
				searching: false,
				onSearch: vi.fn(),
				onClear: vi.fn(),
			},
		});
		expect(page.getByTestId("trail-search-clear").elements()).toHaveLength(0);
	});

	it("renders the clear affordance once a search is active, and clicking it calls onClear", async () => {
		const onClear = vi.fn();
		render(TrailSearch, {
			props: { active: true, searching: false, onSearch: vi.fn(), onClear },
		});
		await expect
			.element(page.getByTestId("trail-search-clear"))
			.toBeInTheDocument();
		await page.getByTestId("trail-search-clear").click();
		expect(onClear).toHaveBeenCalled();
	});

	it("carries the Wave 9 placeholder copy", async () => {
		render(TrailSearch, {
			props: {
				active: false,
				searching: false,
				onSearch: vi.fn(),
				onClear: vi.fn(),
			},
		});
		await expect
			.element(page.getByTestId("trail-search"))
			.toHaveAttribute("placeholder", "Search what you've read…");
	});
});

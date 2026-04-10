import { render } from "vitest-browser-svelte";
import { describe, expect, it } from "vitest";
import SearchSectionSkeleton from "./SearchSectionSkeleton.svelte";

describe("SearchSectionSkeleton Alt-Paper compliance", () => {
	it("renders with data-role attribute", () => {
		render(SearchSectionSkeleton as never, { props: { label: "Loading" } });

		const section = document.querySelector("[data-role='skeleton-section']");
		expect(section).not.toBeNull();
	});

	it("renders correct number of skeleton rows", () => {
		render(SearchSectionSkeleton as never, {
			props: { label: "Loading", rows: 2 },
		});

		const cards = document.querySelectorAll(".skeleton-card");
		expect(cards.length).toBe(2);
	});

	it("does not use rounded classes", () => {
		const { container } = render(SearchSectionSkeleton as never, {
			props: { label: "Loading" },
		});

		const html = container.innerHTML;
		expect(html).not.toContain("rounded-lg");
		expect(html).not.toContain("rounded");
	});
});

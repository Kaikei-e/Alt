import { page } from "@vitest/browser/context";
import { render } from "vitest-browser-svelte";
import { describe, expect, it, vi } from "vitest";
import MobileAcolyteSectionTabs from "./MobileAcolyteSectionTabs.svelte";
import { MOCK_SECTIONS } from "./acolyte-fixtures";

describe("MobileAcolyteSectionTabs", () => {
	it("renders all section tabs", async () => {
		render(MobileAcolyteSectionTabs as never, {
			props: {
				sections: MOCK_SECTIONS,
				activeSection: "overview",
				onSelect: vi.fn(),
			},
		});

		await expect.element(page.getByText("overview")).toBeInTheDocument();
		await expect.element(page.getByText("market trends")).toBeInTheDocument();
		await expect
			.element(page.getByText("technology landscape"))
			.toBeInTheDocument();
	});

	it("renders version badge for each tab", async () => {
		render(MobileAcolyteSectionTabs as never, {
			props: {
				sections: MOCK_SECTIONS,
				activeSection: "overview",
				onSelect: vi.fn(),
			},
		});

		await expect.element(page.getByText("v2")).toBeInTheDocument();
	});

	it("marks active tab", async () => {
		render(MobileAcolyteSectionTabs as never, {
			props: {
				sections: MOCK_SECTIONS,
				activeSection: "overview",
				onSelect: vi.fn(),
			},
		});

		const activeTab = page.getByTestId("section-tab-overview");
		await expect.element(activeTab).toHaveAttribute("data-active", "true");
	});

	it("calls onSelect when tab is clicked", async () => {
		const onSelect = vi.fn();
		render(MobileAcolyteSectionTabs as never, {
			props: {
				sections: MOCK_SECTIONS,
				activeSection: "overview",
				onSelect,
			},
		});

		await page.getByTestId("section-tab-market_trends").click();
		expect(onSelect).toHaveBeenCalledWith("market_trends");
	});
});

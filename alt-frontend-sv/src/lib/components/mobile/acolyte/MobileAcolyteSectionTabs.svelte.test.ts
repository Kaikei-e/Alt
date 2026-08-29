import { page } from "@vitest/browser/context";
import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";
import { MOCK_SECTIONS } from "./acolyte-fixtures";
import MobileAcolyteSectionTabs from "./MobileAcolyteSectionTabs.svelte";

describe("MobileAcolyteSectionTabs", () => {
	it("renders all section tabs", async () => {
		render(MobileAcolyteSectionTabs, {
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
		render(MobileAcolyteSectionTabs, {
			props: {
				sections: MOCK_SECTIONS,
				activeSection: "overview",
				onSelect: vi.fn(),
			},
		});

		await expect.element(page.getByText("v2")).toBeInTheDocument();
	});

	it("marks active tab", async () => {
		render(MobileAcolyteSectionTabs, {
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
		render(MobileAcolyteSectionTabs, {
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

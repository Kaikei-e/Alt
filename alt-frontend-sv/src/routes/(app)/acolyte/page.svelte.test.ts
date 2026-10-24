import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import { MOCK_REPORT_SUMMARIES } from "#lib/components/mobile/acolyte/acolyte-fixtures.js";

const { pageState } = vi.hoisted(() => {
	const value: { url: URL } = {
		url: new URL("http://localhost/acolyte"),
	};
	return {
		pageState: {
			get url() {
				return value.url;
			},
			set url(next: URL) {
				value.url = next;
			},
		},
	};
});

vi.mock("$app/state", () => ({ page: pageState }));

const { gotoMock, listReportsMock } = vi.hoisted(() => ({
	gotoMock: vi.fn(),
	listReportsMock: vi.fn(),
}));

vi.mock("$app/navigation", () => ({
	goto: gotoMock,
}));

vi.mock("#lib/connect/acolyte.js", () => ({
	listReports: listReportsMock,
}));

import Page from "./+page.svelte";

async function settle(ms = 200) {
	await new Promise((resolve) => setTimeout(resolve, ms));
}

describe("Desktop /acolyte/+page.svelte", () => {
	beforeEach(async () => {
		await page.viewport(1280, 800);
		pageState.url = new URL("http://localhost/acolyte");
		gotoMock.mockReset();
		listReportsMock.mockReset();
		listReportsMock.mockResolvedValue({
			reports: MOCK_REPORT_SUMMARIES,
		});
	});

	it("renders .report-card list rows and does NOT render any play or run button on the list view", async () => {
		const { container } = render(Page);
		await settle(250);

		const cards = container.querySelectorAll(".report-card");
		expect(cards.length).toBe(MOCK_REPORT_SUMMARIES.length);

		await expect
			.element(page.getByText("AI Semiconductor Supply Chain Analysis"))
			.toBeInTheDocument();

		const buttons = Array.from(container.querySelectorAll("button, a"));
		for (const btn of buttons) {
			const label = (
				btn.textContent +
				" " +
				(btn.getAttribute("aria-label") ?? "")
			).toLowerCase();
			expect(label).not.toMatch(/\b(play|run|start run|generate)\b/);
		}
	});

	it("renders ?deleted= confirmation banner and allows dismissing it", async () => {
		pageState.url = new URL(
			"http://localhost/acolyte?deleted=Q2+Market+Outlook",
		);

		const { container } = render(Page);
		await settle(250);

		const banner = container.querySelector(".deleted-banner");
		expect(banner).not.toBeNull();
		await expect
			.element(page.getByText("Q2 Market Outlook"))
			.toBeInTheDocument();

		await page.getByRole("button", { name: "Dismiss" }).click();
		await settle(100);

		expect(container.querySelector(".deleted-banner")).toBeNull();
	});
});

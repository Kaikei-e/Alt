import { beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";
import { render } from "vitest-browser-svelte";
import type { EveningPulse, PulseTopic } from "#lib/schema/evening_pulse.js";
import { goto } from "$app/navigation";

vi.mock("$app/navigation", () => ({
	goto: vi.fn(),
}));

const mockTopic = {
	clusterId: 42,
	title: "AI Breakthroughs",
	genre: "technology",
	role: "need_to_know" as const,
	timeAgo: "2 hours ago",
	rationale: { text: "Major advances in chip architecture", confidence: 0.95 },
	sourceNames: ["TechSource"],
	articles: [],
};

const mockPulseData = {
	status: "normal" as const,
	generatedAt: "2026-10-08T00:00:00Z",
	topics: [mockTopic],
	surfaceStatus: {
		generatedAt: "2026-10-08T00:00:00Z",
		windowStart: "2026-10-05T00:00:00Z",
		windowEnd: "2026-10-08T00:00:00Z",
		isStale: false,
		staleAfter: "2026-10-09T06:00:00Z",
		degradationReasons: [],
	},
} as unknown as EveningPulse;

const mockStalePulseData = {
	status: "normal" as const,
	generatedAt: "2026-10-08T00:00:00Z",
	topics: [mockTopic],
	surfaceStatus: {
		generatedAt: "2026-10-08T00:00:00Z",
		windowStart: "2026-10-05T00:00:00Z",
		windowEnd: "2026-10-08T00:00:00Z",
		isStale: true,
		staleAfter: "2026-10-09T06:00:00Z",
		degradationReasons: [
			{
				code: "pulse_fallback",
				detail: "fallback level 2",
			},
		],
	},
} as unknown as EveningPulse;

const mockQuietDayData = {
	status: "quiet_day" as const,
	generatedAt: "2026-10-08T00:00:00Z",
	quietDay: {
		message: "No major updates today",
		weeklyHighlights: [
			{
				id: "hl-42",
				title: "Key Weekly Highlight",
				role: "need_to_know" as const,
				date: "2026-10-07",
			},
		],
	},
	topics: [],
	surfaceStatus: {
		generatedAt: "2026-10-08T00:00:00Z",
		windowStart: "2026-10-05T00:00:00Z",
		windowEnd: "2026-10-08T00:00:00Z",
		isStale: false,
		staleAfter: "2026-10-09T06:00:00Z",
		degradationReasons: [],
	},
} as unknown as EveningPulse;

let currentPulseState = $state({
	data: mockPulseData as EveningPulse | null,
	isLoading: false,
	error: null as Error | null,
	isRetrying: false,
	selectedTopic: null as PulseTopic | null,
	fetchData: vi.fn(),
	retry: vi.fn(),
	selectTopic: vi.fn((clusterId: number) => {
		const t = currentPulseState.data?.topics?.find(
			(x: PulseTopic) => x.clusterId === clusterId,
		);
		currentPulseState.selectedTopic = t ?? null;
		return t ?? null;
	}),
	clearSelectedTopic: vi.fn(() => {
		currentPulseState.selectedTopic = null;
	}),
});

vi.mock("#lib/hooks/usePulse.svelte.js", () => ({
	usePulse: () => currentPulseState,
}));

import Page from "./+page.svelte";

const LANDSCAPE = { width: 1024, height: 768 };
const PORTRAIT = { width: 393, height: 851 };

async function settle() {
	await new Promise((resolve) => setTimeout(resolve, 50));
}

describe("Evening Pulse recap navigation targets", () => {
	beforeEach(async () => {
		vi.mocked(goto).mockReset();
		vi.mocked(goto).mockImplementation(async () => {});
		currentPulseState.data = mockPulseData;
		currentPulseState.selectedTopic = null;
		await page.viewport(LANDSCAPE.width, LANDSCAPE.height);
		await settle();
	});

	it("navigates to a target starting with /recap? when clicking a weekly highlight in quiet day", async () => {
		currentPulseState.data = mockQuietDayData;
		render(Page);
		await settle();

		const highlightButton = page.getByRole("button", {
			name: /Key Weekly Highlight/i,
		});
		await highlightButton.click();

		expect(goto).toHaveBeenCalled();
		const target = String(vi.mocked(goto).mock.calls[0]?.[0] ?? "");
		expect(target.startsWith("/recap?")).toBe(true);
	});

	it("navigates to a target starting with /recap? when clicking view in recap from detail panel", async () => {
		render(Page);
		await settle();

		const topicCard = page.getByRole("button", {
			name: /AI Breakthroughs/i,
		});
		await topicCard.click();
		await settle();

		const recapButton = page.getByRole("button", {
			name: /View in 3-Day Recap/i,
		});
		await recapButton.click();

		expect(goto).toHaveBeenCalled();
		const target = String(vi.mocked(goto).mock.calls[0]?.[0] ?? "");
		expect(target.startsWith("/recap?")).toBe(true);
	});

	it("navigates to a target starting with /recap? on mobile topic sheet", async () => {
		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		render(Page);
		await settle();

		const topicCard = page.getByRole("button", {
			name: /AI Breakthroughs/i,
		});
		await topicCard.click();
		await settle();

		const recapButton = page.getByRole("button", {
			name: /View in 3-Day Recap/i,
		});
		await recapButton.click();

		expect(goto).toHaveBeenCalled();
		const target = String(vi.mocked(goto).mock.calls[0]?.[0] ?? "");
		expect(target.startsWith("/recap?")).toBe(true);
	});

	it("renders the Evening Pulse page heading once on desktop and mobile when current", async () => {
		const desktop = render(Page);
		await settle();

		const desktopHeadings = desktop.container.querySelectorAll("h1");
		expect(desktopHeadings).toHaveLength(1);
		expect(desktopHeadings[0]?.textContent?.trim()).toBe("Evening Pulse");
		expect(desktop.container.textContent).toContain("Today's key topics");
		desktop.unmount();

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		const mobile = render(Page);
		await settle();

		const mobileHeadings = mobile.container.querySelectorAll("h1");
		expect(mobileHeadings).toHaveLength(1);
		expect(mobileHeadings[0]?.textContent?.trim()).toBe("Evening Pulse");
		expect(mobile.container.textContent).toContain("Today's key topics");
	});

	it("replaces 'Today's key topics' with dated subtitle and renders SurfaceStatusBanner when stale", async () => {
		currentPulseState.data = mockStalePulseData;
		const desktop = render(Page);
		await settle();

		expect(desktop.container.textContent).not.toContain("Today's key topics");
		expect(desktop.container.textContent).toContain("Key topics from");
		await expect
			.element(page.getByRole("alert"))
			.toHaveTextContent("This was last updated");
		await expect
			.element(page.getByRole("alert"))
			.toHaveTextContent("fallback level 2");
		desktop.unmount();

		await page.viewport(PORTRAIT.width, PORTRAIT.height);
		const mobile = render(Page);
		await settle();

		expect(mobile.container.textContent).not.toContain("Today's key topics");
		expect(mobile.container.textContent).toContain("Key topics from");
	});
});

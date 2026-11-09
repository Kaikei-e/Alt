import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";
import PulseHeader from "#lib/components/pulse/PulseHeader.svelte";
import PulseWeeklyHighlight from "#lib/components/pulse/PulseWeeklyHighlight.svelte";
import type { PulseTopic } from "#lib/schema/evening_pulse.js";
import DesktopPulseCard from "./DesktopPulseCard.svelte";
import DesktopPulseDetailPanel from "./DesktopPulseDetailPanel.svelte";
import DesktopPulseError from "./DesktopPulseError.svelte";
import DesktopPulseQuietDay from "./DesktopPulseQuietDay.svelte";
import DesktopPulseSkeleton from "./DesktopPulseSkeleton.svelte";

function makeTopic(overrides: Partial<PulseTopic> = {}): PulseTopic {
	return {
		clusterId: 3,
		role: "need_to_know",
		title: "Key News (Cluster 3)",
		rationale: {
			text: "Selected from 6 articles across 4 sources including Tier1.",
			confidence: "high",
		},
		articleCount: 6,
		sourceCount: 4,
		tier1Count: 2,
		timeAgo: "2h ago",
		trendMultiplier: undefined,
		genre: "Technology",
		articleIds: ["art-1", "art-2"],
		representativeArticles: [
			{
				articleId: "art-1",
				title: "New Transformer Architecture Cuts Inference Cost by 60%",
				sourceUrl: "https://example.com/articles/1",
				sourceName: "Ars Technica",
				publishedAt: "2h ago",
			},
			{
				articleId: "art-2",
				title: "Cloud Providers Roll Out Next-Gen Inference Clusters",
				sourceUrl: "https://example.com/articles/2",
				sourceName: "The Register",
				publishedAt: "3h ago",
			},
		],
		topEntities: ["Transformers", "Inference"],
		sourceNames: ["Ars Technica", "The Register"],
		...overrides,
	};
}

describe("DesktopPulseCard", () => {
	it("renders the representative article title as the primary heading and cluster label as kicker", () => {
		const topic = makeTopic();
		const { container } = render(DesktopPulseCard, {
			props: { topic, index: 0 },
		});

		const heading = container.querySelector("h3");
		expect(heading).not.toBeNull();
		expect(heading?.textContent?.trim()).toBe(
			"New Transformer Architecture Cuts Inference Cost by 60%",
		);

		const kicker = container.querySelector('[data-testid="pulse-card-kicker"]');
		expect(kicker).not.toBeNull();
		expect(kicker?.textContent).toContain("Need to know");
		expect(kicker?.textContent).not.toContain("Today's Key");
		expect(kicker?.textContent).toContain("Key News (Cluster 3)");
	});

	it("falls back to topic.title when representativeArticles is empty", () => {
		const topic = makeTopic({ representativeArticles: [] });
		const { container } = render(DesktopPulseCard, {
			props: { topic, index: 0 },
		});

		const heading = container.querySelector("h3");
		expect(heading?.textContent?.trim()).toBe("Key News (Cluster 3)");
	});

	it("applies hover and focused classes without inline background or heading color overrides", () => {
		const topic = makeTopic();
		const unfocused = render(DesktopPulseCard, {
			props: { topic, index: 0, isFocused: false },
		});
		const unfocusedButton = unfocused.container.querySelector("button");
		const unfocusedHeading = unfocused.container.querySelector("h3");
		expect(unfocusedButton?.getAttribute("style") ?? "").not.toContain(
			"background",
		);
		expect(unfocusedHeading?.getAttribute("style") ?? "").not.toContain(
			"color",
		);
		expect(unfocusedButton?.className).toContain("bg-[var(--surface-bg)]");
		expect(unfocusedButton?.className).toContain(
			"hover:bg-[var(--surface-hover)]",
		);
		expect(unfocusedButton?.getAttribute("data-focused")).toBeNull();
		unfocused.unmount();

		const focused = render(DesktopPulseCard, {
			props: { topic, index: 0, isFocused: true },
		});
		const focusedButton = focused.container.querySelector("button");
		expect(focusedButton?.getAttribute("data-focused")).toBe("true");
		expect(focusedButton?.className).toContain("bg-[var(--surface-hover)]");
	});
});

describe("DesktopPulseError", () => {
	it("renders an SVG icon instead of the literal warning character and includes exactly one h1", () => {
		const onRetry = vi.fn();
		const { container } = render(DesktopPulseError, {
			props: {
				error: new Error("Failed to fetch pulse"),
				onRetry,
				isRetrying: false,
			},
		});

		expect(container.querySelector("svg")).not.toBeNull();
		expect(container.textContent).not.toContain("\u26a0");
		expect(container.textContent).not.toContain("\\u26A0");

		const headings = container.querySelectorAll("h1");
		expect(headings).toHaveLength(1);
		expect(headings[0]?.textContent?.trim()).toBe("Evening Pulse");
	});
});

describe("DesktopPulseSkeleton", () => {
	it("renders a single Evening Pulse h1 in loading state", () => {
		const { container } = render(DesktopPulseSkeleton);
		const headings = container.querySelectorAll("h1");
		expect(headings).toHaveLength(1);
		expect(headings[0]?.textContent?.trim()).toBe("Evening Pulse");
	});
});

describe("DesktopPulseQuietDay", () => {
	it("uses neutral default message without Today and labels recap button as 3-Day Recap", () => {
		const { container } = render(DesktopPulseQuietDay, {
			props: {
				date: "2026-10-08",
				quietDay: {
					message: "",
					weeklyHighlights: [],
				},
			},
		});

		expect(container.textContent).toContain(
			"A quiet day. No notable news was found.",
		);
		expect(container.textContent).not.toContain("Today was a quiet day");
	});
});

describe("PulseHeader", () => {
	it("labels the navigation button View 3-Day Recap with visible focus ring and non-repeating dated subtitle", () => {
		const onNavigate = vi.fn();
		const { container } = render(PulseHeader, {
			props: {
				date: "2026-10-08",
				isCurrent: false,
				onNavigateToRecap: onNavigate,
			},
		});

		const button = container.querySelector("button");
		expect(button?.textContent?.trim()).toBe("View 3-Day Recap");
		expect(button?.className).toContain("focus-visible:outline-2");
		expect(container.textContent).toContain("Key topics curated for you");
		expect(container.textContent).not.toContain("Key topics from");
	});
});

describe("PulseWeeklyHighlight", () => {
	it("uses class-based background/color and visible focus ring instead of inline overrides", () => {
		const { container } = render(PulseWeeklyHighlight, {
			props: {
				highlight: {
					id: "hl-1",
					title: "Highlight Title",
					date: "2026-10-07",
					role: "need_to_know",
				},
			},
		});

		const button = container.querySelector("button");
		const heading = container.querySelector("h4");
		expect(button?.getAttribute("style") ?? "").not.toContain("background");
		expect(heading?.getAttribute("style") ?? "").not.toContain("color");
		expect(button?.className).toContain("bg-[var(--surface-bg)]");
		expect(button?.className).toContain("hover:bg-[var(--surface-hover)]");
		expect(button?.className).toContain("focus-visible:outline-2");
	});
});

describe("DesktopPulseDetailPanel", () => {
	it("applies shadow-none to Sheet.Content and visible focus outline to Sheet.Close", () => {
		const topic = makeTopic();
		render(DesktopPulseDetailPanel, {
			props: {
				topic,
				open: true,
				onClose: vi.fn(),
				onNavigateToRecap: vi.fn(),
			},
		});

		const dialog = document.querySelector('[data-slot="sheet-content"]');
		expect(dialog?.className).toContain("shadow-none");

		const closeBtn = document.querySelector('[data-slot="sheet-close"]');
		expect(closeBtn?.className).toContain("focus-visible:outline-2");
		expect(closeBtn?.className).not.toContain("focus-visible:outline-none");
	});
});

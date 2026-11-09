import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";
import type { PulseTopic } from "#lib/schema/evening_pulse.js";
import MobilePulseCard from "./MobilePulseCard.svelte";
import MobilePulseError from "./MobilePulseError.svelte";

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
		articleIds: ["art-1"],
		representativeArticles: [
			{
				articleId: "art-1",
				title: "New Transformer Architecture Cuts Inference Cost by 60%",
				sourceUrl: "https://example.com/articles/1",
				sourceName: "Ars Technica",
				publishedAt: "2h ago",
			},
		],
		topEntities: ["Transformers", "Inference"],
		sourceNames: ["Ars Technica"],
		...overrides,
	};
}

describe("MobilePulseCard", () => {
	it("renders the representative article title as the primary heading and cluster label as kicker", () => {
		const topic = makeTopic();
		const { container } = render(MobilePulseCard, {
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
		const { container } = render(MobilePulseCard, {
			props: { topic, index: 0 },
		});

		const heading = container.querySelector("h3");
		expect(heading?.textContent?.trim()).toBe("Key News (Cluster 3)");
	});
});

describe("MobilePulseError", () => {
	it("renders an SVG icon instead of the literal warning character and uses high-contrast button tokens", () => {
		const onRetry = vi.fn();
		const { container } = render(MobilePulseError, {
			props: {
				error: new Error("Failed to fetch pulse"),
				onRetry,
				isRetrying: false,
			},
		});

		expect(container.querySelector("svg")).not.toBeNull();
		expect(container.textContent).not.toContain("\u26a0");
		expect(container.textContent).not.toContain("\\u26A0");

		const retryButton = container.querySelector("button");
		expect(retryButton).not.toBeNull();
		const buttonStyle = retryButton?.getAttribute("style") ?? "";
		expect(buttonStyle).not.toContain("color: var(--text-primary)");
	});
});

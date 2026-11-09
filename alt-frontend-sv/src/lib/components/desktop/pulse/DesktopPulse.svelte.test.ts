import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-svelte";
import type { PulseTopic } from "#lib/schema/evening_pulse.js";
import DesktopPulseCard from "./DesktopPulseCard.svelte";
import DesktopPulseError from "./DesktopPulseError.svelte";

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
});

describe("DesktopPulseError", () => {
	it("renders an SVG icon instead of the literal warning character", () => {
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
	});
});

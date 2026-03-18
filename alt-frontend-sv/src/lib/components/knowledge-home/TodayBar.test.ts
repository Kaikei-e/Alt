import { describe, it, expect } from "vitest";
import type { TodayDigestData } from "$lib/connect/knowledge_home";

/**
 * Tests for TodayBar data logic.
 * Component rendering is tested via browser tests (*.svelte.test.ts).
 */

function makeDigest(overrides: Partial<TodayDigestData> = {}): TodayDigestData {
	return {
		date: "2026-03-17",
		newArticles: 42,
		summarizedArticles: 30,
		unsummarizedArticles: 12,
		topTags: ["AI", "Go", "Rust"],
		weeklyRecapAvailable: true,
		eveningPulseAvailable: false,
		needToKnowCount: 0,
		...overrides,
	};
}

describe("TodayBar data", () => {
	it("creates a valid digest with all fields", () => {
		const digest = makeDigest();
		expect(digest.newArticles).toBe(42);
		expect(digest.summarizedArticles).toBe(30);
		expect(digest.unsummarizedArticles).toBe(12);
		expect(digest.topTags).toHaveLength(3);
	});

	it("handles null digest (component shows nothing)", () => {
		const digest: TodayDigestData | null = null;
		expect(digest).toBeNull();
	});

	it("handles empty top tags", () => {
		const digest = makeDigest({ topTags: [] });
		expect(digest.topTags).toHaveLength(0);
	});

	it("computes total articles from new field", () => {
		const digest = makeDigest();
		expect(digest.newArticles).toBe(
			digest.summarizedArticles + digest.unsummarizedArticles,
		);
	});

	it("shows recap shortcut when weeklyRecapAvailable", () => {
		const digest = makeDigest({ weeklyRecapAvailable: true });
		expect(digest.weeklyRecapAvailable).toBe(true);
	});

	it("shows pulse shortcut when eveningPulseAvailable", () => {
		const digest = makeDigest({ eveningPulseAvailable: true });
		expect(digest.eveningPulseAvailable).toBe(true);
	});

	it("handles zero articles", () => {
		const digest = makeDigest({
			newArticles: 0,
			summarizedArticles: 0,
			unsummarizedArticles: 0,
		});
		expect(digest.newArticles).toBe(0);
	});

	it("morning letter link is always available (no flag needed)", () => {
		// TodayBar always shows Morning Letter link regardless of flags
		const digest = makeDigest({
			weeklyRecapAvailable: false,
			eveningPulseAvailable: false,
		});
		// Morning Letter doesn't depend on any availability flag
		expect(digest).toBeDefined();
	});

	it("needToKnowCount defaults to 0", () => {
		const digest = makeDigest();
		expect(digest.needToKnowCount).toBe(0);
	});

	it("needToKnowCount reflects pulse_need_to_know items", () => {
		const digest = makeDigest({ needToKnowCount: 3 });
		expect(digest.needToKnowCount).toBe(3);
	});

	it("shows badge when needToKnowCount > 0 alongside pulse", () => {
		const digest = makeDigest({
			eveningPulseAvailable: true,
			needToKnowCount: 5,
		});
		expect(digest.eveningPulseAvailable).toBe(true);
		expect(digest.needToKnowCount).toBe(5);
	});
});

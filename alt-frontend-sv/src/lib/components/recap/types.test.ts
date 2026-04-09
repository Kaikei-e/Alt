import { describe, expect, it } from "vitest";
import { fromGlobalRecapHit, fromRecapSearchResult } from "./types";
import type { GlobalRecapHitData } from "$lib/connect/global_search";
import type { RecapSearchResultItem } from "$lib/connect";

describe("fromGlobalRecapHit", () => {
	const hit: GlobalRecapHitData = {
		id: "hit-1",
		jobId: "job-abc",
		genre: "Technology",
		summary: "AI advances in 2026",
		topTerms: ["LLM", "agents"],
		tags: ["ai", "tech"],
		windowDays: 3,
		executedAt: "2026-04-08T12:00:00Z",
	};

	it("maps shared fields correctly", () => {
		const result = fromGlobalRecapHit(hit);
		expect(result.genre).toBe("Technology");
		expect(result.summary).toBe("AI advances in 2026");
		expect(result.topTerms).toEqual(["LLM", "agents"]);
		expect(result.windowDays).toBe(3);
		expect(result.executedAt).toBe("2026-04-08T12:00:00Z");
		expect(result.jobId).toBe("job-abc");
	});

	it("maps tags from the hit", () => {
		const result = fromGlobalRecapHit(hit);
		expect(result.tags).toEqual(["ai", "tech"]);
	});

	it("sets bullets to undefined", () => {
		const result = fromGlobalRecapHit(hit);
		expect(result.bullets).toBeUndefined();
	});
});

describe("fromRecapSearchResult", () => {
	const item: RecapSearchResultItem = {
		jobId: "job-xyz",
		executedAt: "2026-04-07T09:00:00Z",
		windowDays: 7,
		genre: "Politics",
		summary: "Election coverage summary",
		topTerms: ["election", "policy"],
		bullets: ["Point A", "Point B"],
	};

	it("maps shared fields correctly", () => {
		const result = fromRecapSearchResult(item);
		expect(result.genre).toBe("Politics");
		expect(result.summary).toBe("Election coverage summary");
		expect(result.topTerms).toEqual(["election", "policy"]);
		expect(result.windowDays).toBe(7);
		expect(result.executedAt).toBe("2026-04-07T09:00:00Z");
		expect(result.jobId).toBe("job-xyz");
	});

	it("maps bullets from the item", () => {
		const result = fromRecapSearchResult(item);
		expect(result.bullets).toEqual(["Point A", "Point B"]);
	});

	it("sets tags to undefined", () => {
		const result = fromRecapSearchResult(item);
		expect(result.tags).toBeUndefined();
	});
});

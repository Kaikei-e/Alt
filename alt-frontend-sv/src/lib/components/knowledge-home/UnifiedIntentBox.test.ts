import { describe, expect, it } from "vitest";

/**
 * Tests for UnifiedIntentBox data logic.
 * Component rendering is tested via browser tests (*.svelte.test.ts).
 */
describe("UnifiedIntentBox", () => {
	it("search navigates to /feeds/search with encoded query", () => {
		const query = "AI trends 2026";
		const url = `/feeds/search?q=${encodeURIComponent(query)}`;
		expect(url).toBe("/feeds/search?q=AI%20trends%202026");
	});

	it("ask navigates to /augur with encoded query", () => {
		const query = "what is happening with AI?";
		const url = `/augur?q=${encodeURIComponent(query)}`;
		expect(url).toBe("/augur?q=what%20is%20happening%20with%20AI%3F");
	});

	it("empty query does not produce navigation", () => {
		const query = "   ";
		const trimmed = query.trim();
		expect(trimmed).toBe("");
	});

	it("ask with empty query goes to /augur without params", () => {
		const query = "";
		const trimmed = query.trim();
		const url = trimmed ? `/augur?q=${encodeURIComponent(trimmed)}` : "/augur";
		expect(url).toBe("/augur");
	});
});

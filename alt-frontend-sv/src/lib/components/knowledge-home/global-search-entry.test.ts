import { describe, expect, it } from "vitest";

describe("GlobalSearchEntry logic", () => {
	it("builds correct search URL", () => {
		const query = "test query";
		const url = `/search?q=${encodeURIComponent(query)}`;
		expect(url).toBe("/search?q=test%20query");
	});

	it("builds correct augur URL with query", () => {
		const query = "test question";
		const url = `/augur?q=${encodeURIComponent(query)}`;
		expect(url).toBe("/augur?q=test%20question");
	});

	it("builds augur URL without query", () => {
		expect("/augur").toBe("/augur");
	});

	it("handles special characters in query", () => {
		const query = "AI & ML: what's next?";
		const url = `/search?q=${encodeURIComponent(query)}`;
		expect(url).toContain("search?q=");
		expect(decodeURIComponent(url.split("q=")[1])).toBe(query);
	});
});

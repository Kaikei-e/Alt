import { describe, expect, it } from "vitest";
import { resolveResponsiveRedirect } from "./redirect-resolver";

describe("resolveResponsiveRedirect", () => {
	it("preserves query params for /desktop/feeds/search", () => {
		expect(resolveResponsiveRedirect("/desktop/feeds/search", "?q=LLM")).toBe(
			"/feeds/search?q=LLM",
		);
	});

	it("preserves encoded query params for /mobile/feeds/search", () => {
		expect(
			resolveResponsiveRedirect("/mobile/feeds/search", "?q=AI%20trends"),
		).toBe("/feeds/search?q=AI%20trends");
	});

	it("returns target without query when search is empty", () => {
		expect(resolveResponsiveRedirect("/desktop/feeds", "")).toBe("/feeds");
	});

	it("returns null for retired evening pulse and 7-day recap routes", () => {
		expect(
			resolveResponsiveRedirect("/desktop/recap/evening-pulse", ""),
		).toBeNull();
		expect(
			resolveResponsiveRedirect("/mobile/recap/evening-pulse", ""),
		).toBeNull();
		expect(resolveResponsiveRedirect("/mobile/recap/7days", "")).toBeNull();
	});

	it("returns null for unknown paths", () => {
		expect(resolveResponsiveRedirect("/nonexistent", "?q=test")).toBeNull();
	});

	it("preserves multiple query params", () => {
		expect(
			resolveResponsiveRedirect("/desktop/feeds/search", "?q=LLM&page=2"),
		).toBe("/feeds/search?q=LLM&page=2");
	});
});

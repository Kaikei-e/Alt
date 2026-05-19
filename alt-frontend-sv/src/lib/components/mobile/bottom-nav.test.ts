import { describe, expect, it } from "vitest";
import { NAV_TABS, getActiveTabIndex } from "./bottom-nav";

describe("NAV_TABS (re-export)", () => {
	it("has exactly 5 tabs", () => {
		expect(NAV_TABS).toHaveLength(5);
	});

	it("has Home, Loop, Search, Augur, Menu in order", () => {
		expect(NAV_TABS.map((t) => t.label)).toEqual([
			"Home",
			"Loop",
			"Search",
			"Augur",
			"Menu",
		]);
	});
});

describe("getActiveTabIndex", () => {
	it("returns 0 for /home and sub-paths", () => {
		expect(getActiveTabIndex("/home")).toBe(0);
		expect(getActiveTabIndex("/home/digest")).toBe(0);
	});

	it("returns 1 for /loop root and sub-paths", () => {
		expect(getActiveTabIndex("/loop")).toBe(1);
		expect(getActiveTabIndex("/loop/welcome")).toBe(1);
	});

	it("returns 2 for /search and /feeds/search", () => {
		expect(getActiveTabIndex("/search")).toBe(2);
		expect(getActiveTabIndex("/feeds/search")).toBe(2);
		expect(getActiveTabIndex("/feeds/search/results")).toBe(2);
	});

	it("returns 3 for /augur and sub-paths", () => {
		expect(getActiveTabIndex("/augur")).toBe(3);
		expect(getActiveTabIndex("/augur/history")).toBe(3);
		expect(getActiveTabIndex("/augur/abc123")).toBe(3);
	});

	it("returns 4 for /menu", () => {
		expect(getActiveTabIndex("/menu")).toBe(4);
	});

	it("falls back to Menu (4) for /feeds (now a secondary destination)", () => {
		expect(getActiveTabIndex("/feeds")).toBe(4);
		expect(getActiveTabIndex("/feeds/swipe")).toBe(4);
		expect(getActiveTabIndex("/feeds/swipe/visual-preview")).toBe(4);
		expect(getActiveTabIndex("/feeds/favorites")).toBe(4);
		expect(getActiveTabIndex("/feeds/viewed")).toBe(4);
		expect(getActiveTabIndex("/feeds/tag-trail")).toBe(4);
	});

	it("falls back to Menu (4) for secondary destinations surfaced by the Menu page", () => {
		expect(getActiveTabIndex("/recap")).toBe(4);
		expect(getActiveTabIndex("/recap/morning-letter")).toBe(4);
		expect(getActiveTabIndex("/acolyte")).toBe(4);
		expect(getActiveTabIndex("/settings/feeds")).toBe(4);
		expect(getActiveTabIndex("/stats")).toBe(4);
		expect(getActiveTabIndex("/admin/scraping-domains")).toBe(4);
	});
});

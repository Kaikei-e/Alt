import { describe, expect, it } from "vitest";
import {
	NAV_TABS,
	shouldShowBottomNav,
	getActiveTabIndex,
} from "./bottom-nav";

describe("NAV_TABS", () => {
	it("has exactly 5 tabs", () => {
		expect(NAV_TABS).toHaveLength(5);
	});

	it("has correct labels", () => {
		const labels = NAV_TABS.map((t) => t.label);
		expect(labels).toEqual(["Home", "Swipe", "Search", "Recap", "Library"]);
	});

	it("has correct hrefs", () => {
		const hrefs = NAV_TABS.map((t) => t.href);
		expect(hrefs).toEqual([
			"/home",
			"/feeds/swipe",
			"/search",
			"/recap",
			"/feeds",
		]);
	});
});

describe("shouldShowBottomNav", () => {
	it("returns true for normal paths", () => {
		expect(shouldShowBottomNav("/home")).toBe(true);
		expect(shouldShowBottomNav("/feeds")).toBe(true);
		expect(shouldShowBottomNav("/recap")).toBe(true);
		expect(shouldShowBottomNav("/search")).toBe(true);
	});

	it("returns true for /feeds/swipe (swipe is a primary tab)", () => {
		expect(shouldShowBottomNav("/feeds/swipe")).toBe(true);
	});

	it("returns false for /augur", () => {
		expect(shouldShowBottomNav("/augur")).toBe(false);
	});

	it("returns false for /feeds/search", () => {
		expect(shouldShowBottomNav("/feeds/search")).toBe(false);
	});
});

describe("getActiveTabIndex", () => {
	it("returns 0 for /home", () => {
		expect(getActiveTabIndex("/home")).toBe(0);
	});

	it("returns 1 for /feeds/swipe (Swipe)", () => {
		expect(getActiveTabIndex("/feeds/swipe")).toBe(1);
	});

	it("returns 1 for /feeds/swipe sub-paths", () => {
		expect(getActiveTabIndex("/feeds/swipe/visual-preview")).toBe(1);
	});

	it("returns 2 for /search", () => {
		expect(getActiveTabIndex("/search")).toBe(2);
	});

	it("returns 3 for /recap", () => {
		expect(getActiveTabIndex("/recap")).toBe(3);
	});

	it("returns 3 for /recap sub-paths", () => {
		expect(getActiveTabIndex("/recap/morning-letter")).toBe(3);
	});

	it("returns 4 for /feeds (Library)", () => {
		expect(getActiveTabIndex("/feeds")).toBe(4);
	});

	it("returns 4 for /feeds sub-paths that are not swipe", () => {
		expect(getActiveTabIndex("/feeds/favorites")).toBe(4);
	});

	it("returns -1 for unknown paths", () => {
		expect(getActiveTabIndex("/settings")).toBe(-1);
		expect(getActiveTabIndex("/augur")).toBe(-1);
	});
});

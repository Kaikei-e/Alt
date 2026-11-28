import { describe, expect, it } from "vitest";
import { getActiveTabIndex, isImmersiveRoute, NAV_TABS } from "./bottom-nav";

describe("NAV_TABS (re-export)", () => {
	it("has exactly 5 tabs", () => {
		expect(NAV_TABS).toHaveLength(5);
	});

	it("has Today, Swipe, Gallery, Themes, Menu in order", () => {
		expect(NAV_TABS.map((t) => t.label)).toEqual([
			"Today",
			"Swipe",
			"Gallery",
			"Themes",
			"Menu",
		]);
	});
});

describe("getActiveTabIndex", () => {
	it("returns 0 for /today and sub-paths", () => {
		expect(getActiveTabIndex("/today")).toBe(0);
		expect(getActiveTabIndex("/today/archive")).toBe(0);
	});

	it("returns 1 for /feeds/swipe and sub-paths", () => {
		expect(getActiveTabIndex("/feeds/swipe")).toBe(1);
		expect(getActiveTabIndex("/feeds/swipe/visual-preview")).toBe(1);
	});

	it("returns 2 for /feeds/visual-preview and sub-paths", () => {
		expect(getActiveTabIndex("/feeds/visual-preview")).toBe(2);
		expect(getActiveTabIndex("/feeds/visual-preview/detail")).toBe(2);
	});

	it("returns 3 for /knowledge and sub-paths", () => {
		expect(getActiveTabIndex("/knowledge")).toBe(3);
		expect(
			getActiveTabIndex(
				"/knowledge/themes/01938e82-7c00-7a7b-9b10-000000000001",
			),
		).toBe(3);
	});

	it("returns 4 for /menu", () => {
		expect(getActiveTabIndex("/menu")).toBe(4);
	});

	it("falls back to Menu (4) for secondary destinations surfaced by the Menu page", () => {
		expect(getActiveTabIndex("/search")).toBe(4);
		expect(getActiveTabIndex("/feeds/search")).toBe(4);
		expect(getActiveTabIndex("/augur")).toBe(4);
		expect(getActiveTabIndex("/augur/history")).toBe(4);
		expect(getActiveTabIndex("/feeds")).toBe(4);
		expect(getActiveTabIndex("/feeds/favorites")).toBe(4);
		expect(getActiveTabIndex("/feeds/viewed")).toBe(4);
		expect(getActiveTabIndex("/feeds/tag-trail")).toBe(4);
		expect(getActiveTabIndex("/recap")).toBe(4);
		expect(getActiveTabIndex("/recap/morning-letter")).toBe(4);
		expect(getActiveTabIndex("/acolyte")).toBe(4);
		expect(getActiveTabIndex("/settings/feeds")).toBe(4);
		expect(getActiveTabIndex("/stats")).toBe(4);
		expect(getActiveTabIndex("/admin/monitor")).toBe(4);
	});
});

describe("isImmersiveRoute", () => {
	it("returns true for swipe dispatch surfaces", () => {
		expect(isImmersiveRoute("/feeds/swipe")).toBe(true);
		expect(isImmersiveRoute("/feeds/swipe/visual-preview")).toBe(true);
	});

	it("returns false for every other surface", () => {
		expect(isImmersiveRoute("/today")).toBe(false);
		expect(isImmersiveRoute("/home")).toBe(false);
		expect(isImmersiveRoute("/feeds")).toBe(false);
		expect(isImmersiveRoute("/feeds/visual-preview")).toBe(false);
		expect(isImmersiveRoute("/feeds/favorites")).toBe(false);
		expect(isImmersiveRoute("/menu")).toBe(false);
		expect(isImmersiveRoute("/knowledge")).toBe(false);
		expect(isImmersiveRoute("/knowledge/themes/123")).toBe(false);
		expect(isImmersiveRoute("/knowledge/trail")).toBe(false);
	});

	it("does not treat swipe-prefixed sibling paths as immersive", () => {
		expect(isImmersiveRoute("/feeds/swipeX")).toBe(false);
	});
});

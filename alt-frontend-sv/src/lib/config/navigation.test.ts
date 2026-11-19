import { Layers } from "@lucide/svelte";
import { describe, expect, it } from "vitest";
import {
	getVisibleMobileMenuSections,
	MOBILE_MENU_SECTIONS,
	NAV_TABS,
} from "./navigation";

describe("NAV_TABS", () => {
	it("has exactly 5 tabs", () => {
		expect(NAV_TABS).toHaveLength(5);
	});

	it("has Today, Swipe, Gallery, Search, Menu in order", () => {
		expect(NAV_TABS.map((t) => t.label)).toEqual([
			"Today",
			"Swipe",
			"Gallery",
			"Search",
			"Menu",
		]);
	});

	it("maps labels to expected hrefs", () => {
		expect(NAV_TABS.map((t) => t.href)).toEqual([
			"/today",
			"/feeds/swipe/visual-preview",
			"/feeds/visual-preview",
			"/search",
			"/menu",
		]);
	});

	it("assigns an icon component to every tab", () => {
		for (const tab of NAV_TABS) {
			expect(tab.icon).toBeDefined();
		}
	});

	it("uses Layers icon for Swipe tab", () => {
		const swipeTab = NAV_TABS.find((t) => t.label === "Swipe");
		expect(swipeTab?.icon).toBe(Layers);
	});
});

describe("MOBILE_MENU_SECTIONS", () => {
	it("exposes non-empty sections", () => {
		expect(MOBILE_MENU_SECTIONS.length).toBeGreaterThan(0);
		for (const section of MOBILE_MENU_SECTIONS) {
			expect(section.items.length).toBeGreaterThan(0);
		}
	});

	it("includes moved primary routes (Home, Trail, Augur) and omits Swipe and Gallery", () => {
		const hrefs = MOBILE_MENU_SECTIONS.flatMap((s) =>
			s.items.map((i) => i.href),
		);
		expect(hrefs).toContain("/home");
		expect(hrefs).toContain("/knowledge/trail");
		expect(hrefs).toContain("/augur");
		expect(hrefs).not.toContain("/feeds/swipe/visual-preview");
		expect(hrefs).not.toContain("/feeds/visual-preview");
		expect(hrefs).not.toContain("/today");
		expect(hrefs).not.toContain("/feeds/swipe");
		expect(hrefs).toContain("/recap");
		expect(hrefs).toContain("/recap/morning-letter");
		expect(hrefs).not.toContain("/recap/evening-pulse");
		expect(hrefs).toContain("/recap/job-status");
		expect(hrefs).toContain("/feeds");
		expect(hrefs).toContain("/feeds/favorites");
		expect(hrefs).toContain("/feeds/viewed");
		expect(hrefs).toContain("/feeds/tag-trail");
		expect(hrefs).toContain("/acolyte");
		expect(hrefs).toContain("/augur/history");
		expect(hrefs).toContain("/stats");
		expect(hrefs).toContain("/settings/feeds");
		expect(hrefs).toContain("/admin/monitor");
	});

	it("does not duplicate primary tab destinations", () => {
		const tabHrefs = new Set(NAV_TABS.map((t) => t.href));
		const menuHrefs = MOBILE_MENU_SECTIONS.flatMap((s) =>
			s.items.map((i) => i.href),
		);
		for (const href of menuHrefs) {
			expect(tabHrefs.has(href)).toBe(false);
		}
	});

	it("has no duplicate hrefs across all menu items", () => {
		const menuHrefs = MOBILE_MENU_SECTIONS.flatMap((s) =>
			s.items.map((i) => i.href),
		);
		expect(new Set(menuHrefs).size).toBe(menuHrefs.length);
	});

	it("does not surface the retired Knowledge Loop route anywhere in the menu", () => {
		const menuHrefs = MOBILE_MENU_SECTIONS.flatMap((s) =>
			s.items.map((i) => i.href),
		);
		expect(menuHrefs).not.toContain("/loop");
	});
});

describe("getVisibleMobileMenuSections", () => {
	it("hides requiresAdmin items when not admin", () => {
		const sections = getVisibleMobileMenuSections(false);
		const adminItems = sections
			.flatMap((s) => s.items)
			.filter((i) => i.requiresAdmin);
		expect(adminItems).toHaveLength(0);
	});

	it("reveals requiresAdmin items when admin", () => {
		const visible = getVisibleMobileMenuSections(true);
		const all = MOBILE_MENU_SECTIONS.flatMap((s) => s.items);
		const visibleAll = visible.flatMap((s) => s.items);
		expect(visibleAll).toHaveLength(all.length);
	});

	it("drops sections that become empty after filtering", () => {
		const nonAdmin = getVisibleMobileMenuSections(false);
		for (const section of nonAdmin) {
			expect(section.items.length).toBeGreaterThan(0);
		}
	});
});

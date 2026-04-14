import { describe, expect, it } from "vitest";
import {
	NAV_TABS,
	MOBILE_MENU_SECTIONS,
	getVisibleMobileMenuSections,
} from "./navigation";

describe("NAV_TABS", () => {
	it("has exactly 5 tabs", () => {
		expect(NAV_TABS).toHaveLength(5);
	});

	it("has Home, Swipe, Search, Augur, Menu in order", () => {
		expect(NAV_TABS.map((t) => t.label)).toEqual([
			"Home",
			"Swipe",
			"Search",
			"Augur",
			"Menu",
		]);
	});

	it("maps labels to expected hrefs", () => {
		expect(NAV_TABS.map((t) => t.href)).toEqual([
			"/home",
			"/feeds/swipe/visual-preview",
			"/search",
			"/augur",
			"/menu",
		]);
	});

	it("assigns an icon component to every tab", () => {
		for (const tab of NAV_TABS) {
			expect(tab.icon).toBeDefined();
		}
	});
});

describe("MOBILE_MENU_SECTIONS", () => {
	it("exposes non-empty sections", () => {
		expect(MOBILE_MENU_SECTIONS.length).toBeGreaterThan(0);
		for (const section of MOBILE_MENU_SECTIONS) {
			expect(section.items.length).toBeGreaterThan(0);
		}
	});

	it("absorbs the FloatingMenu orphan items", () => {
		const hrefs = MOBILE_MENU_SECTIONS.flatMap((s) =>
			s.items.map((i) => i.href),
		);
		expect(hrefs).toContain("/recap");
		expect(hrefs).toContain("/recap/morning-letter");
		expect(hrefs).toContain("/recap/evening-pulse");
		expect(hrefs).toContain("/recap/job-status");
		expect(hrefs).toContain("/feeds/swipe");
		expect(hrefs).toContain("/feeds");
		expect(hrefs).toContain("/feeds/favorites");
		expect(hrefs).toContain("/feeds/viewed");
		expect(hrefs).toContain("/feeds/tag-trail");
		expect(hrefs).toContain("/acolyte");
		expect(hrefs).toContain("/augur/history");
		expect(hrefs).toContain("/stats");
		expect(hrefs).toContain("/settings/feeds");
		expect(hrefs).toContain("/admin/scraping-domains");
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

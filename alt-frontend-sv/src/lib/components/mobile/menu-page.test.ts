import { describe, expect, it } from "vitest";
import { MENU_SECTIONS, getVisibleSections } from "./menu-page";

describe("MENU_SECTIONS", () => {
	it("has 4 sections", () => {
		expect(MENU_SECTIONS).toHaveLength(4);
	});

	it("has correct section titles", () => {
		const titles = MENU_SECTIONS.map((s) => s.title);
		expect(titles).toEqual(["Browse", "AI & Insights", "Settings", "Admin"]);
	});

	it("Browse section has 6 items", () => {
		const browse = MENU_SECTIONS.find((s) => s.title === "Browse");
		expect(browse?.items).toHaveLength(6);
	});

	it("AI & Insights section has 4 items", () => {
		const ai = MENU_SECTIONS.find((s) => s.title === "AI & Insights");
		expect(ai?.items).toHaveLength(4);
	});

	it("Settings section has 1 item", () => {
		const settings = MENU_SECTIONS.find((s) => s.title === "Settings");
		expect(settings?.items).toHaveLength(1);
	});

	it("Visual Preview points to /feeds/swipe/visual-preview", () => {
		const browse = MENU_SECTIONS.find((s) => s.title === "Browse");
		const vp = browse?.items.find((i) => i.label === "Visual Preview");
		expect(vp?.href).toBe("/feeds/swipe/visual-preview");
	});

	it("does not contain Settings item", () => {
		const allItems = MENU_SECTIONS.flatMap((s) => s.items);
		expect(allItems.find((i) => i.label === "Settings")).toBeUndefined();
	});

	it("Admin section has 1 item", () => {
		const admin = MENU_SECTIONS.find((s) => s.title === "Admin");
		expect(admin?.items).toHaveLength(1);
	});

	it("Library points to /feeds", () => {
		const browse = MENU_SECTIONS.find((s) => s.title === "Browse");
		const library = browse?.items.find((i) => i.label === "Library");
		expect(library?.href).toBe("/feeds");
	});

	it("Tag Verse has Desktop badge", () => {
		const browse = MENU_SECTIONS.find((s) => s.title === "Browse");
		const tagVerse = browse?.items.find((i) => i.label === "Tag Verse");
		expect(tagVerse?.badge).toBe("Desktop");
	});

	it("Admin item requires admin role", () => {
		const admin = MENU_SECTIONS.find((s) => s.title === "Admin");
		expect(admin?.items[0]?.requiresAdmin).toBe(true);
	});
});

describe("getVisibleSections", () => {
	it("excludes Admin section when not admin", () => {
		const sections = getVisibleSections(false);
		expect(sections).toHaveLength(3);
		expect(sections.find((s) => s.title === "Admin")).toBeUndefined();
	});

	it("includes all 4 sections when admin", () => {
		const sections = getVisibleSections(true);
		expect(sections).toHaveLength(4);
		expect(sections.find((s) => s.title === "Admin")).toBeDefined();
	});

	it("preserves non-admin items in all sections", () => {
		const sections = getVisibleSections(false);
		const browse = sections.find((s) => s.title === "Browse");
		expect(browse?.items).toHaveLength(6);
	});
});

import { describe, expect, it } from "vitest";
import { MORE_SHEET_ITEMS, getVisibleItems } from "./more-sheet";

describe("MORE_SHEET_ITEMS", () => {
	it("has 8 items total", () => {
		expect(MORE_SHEET_ITEMS).toHaveLength(8);
	});

	it("has correct labels", () => {
		const labels = MORE_SHEET_ITEMS.map((i) => i.label);
		expect(labels).toEqual([
			"Ask Augur",
			"Explore",
			"Settings",
			"Manage Feeds",
			"Statistics",
			"Job Status",
			"Tag Verse",
			"Admin",
		]);
	});

	it("Tag Verse has Desktop badge", () => {
		const tagVerse = MORE_SHEET_ITEMS.find((i) => i.label === "Tag Verse");
		expect(tagVerse?.badge).toBe("Desktop");
	});

	it("Admin requires admin role", () => {
		const admin = MORE_SHEET_ITEMS.find((i) => i.label === "Admin");
		expect(admin?.requiresAdmin).toBe(true);
	});

	it("Explore points to /feeds/tag-trail", () => {
		const explore = MORE_SHEET_ITEMS.find((i) => i.label === "Explore");
		expect(explore?.href).toBe("/feeds/tag-trail");
	});
});

describe("getVisibleItems", () => {
	it("excludes Admin when not admin", () => {
		const items = getVisibleItems(false);
		expect(items).toHaveLength(7);
		expect(items.find((i) => i.label === "Admin")).toBeUndefined();
	});

	it("includes Admin when admin", () => {
		const items = getVisibleItems(true);
		expect(items).toHaveLength(8);
		expect(items.find((i) => i.label === "Admin")).toBeDefined();
	});
});

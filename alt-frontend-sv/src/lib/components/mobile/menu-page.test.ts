import { describe, expect, it } from "vitest";
import { MENU_SECTIONS, getVisibleSections } from "./menu-page";

describe("MENU_SECTIONS (re-export of MOBILE_MENU_SECTIONS)", () => {
	it("has at least one section", () => {
		expect(MENU_SECTIONS.length).toBeGreaterThan(0);
	});

	it("includes a Browse section", () => {
		expect(MENU_SECTIONS.find((s) => s.title === "Browse")).toBeDefined();
	});

	it("includes a Recap section", () => {
		expect(MENU_SECTIONS.find((s) => s.title === "Recap")).toBeDefined();
	});

	it("includes an AI & Insights section", () => {
		expect(
			MENU_SECTIONS.find((s) => s.title === "AI & Insights"),
		).toBeDefined();
	});

	it("includes a Settings section", () => {
		expect(MENU_SECTIONS.find((s) => s.title === "Settings")).toBeDefined();
	});

	it("includes an Admin section", () => {
		expect(MENU_SECTIONS.find((s) => s.title === "Admin")).toBeDefined();
	});

	it("Browse includes Swipe Mode pointing to /feeds/swipe", () => {
		const browse = MENU_SECTIONS.find((s) => s.title === "Browse");
		const swipe = browse?.items.find((i) => i.href === "/feeds/swipe");
		expect(swipe).toBeDefined();
	});

	it("Recap includes Job Status pointing to /recap/job-status", () => {
		const recap = MENU_SECTIONS.find((s) => s.title === "Recap");
		const job = recap?.items.find((i) => i.href === "/recap/job-status");
		expect(job).toBeDefined();
	});

	it("AI & Insights includes Augur History pointing to /augur/history", () => {
		const ai = MENU_SECTIONS.find((s) => s.title === "AI & Insights");
		const hist = ai?.items.find((i) => i.href === "/augur/history");
		expect(hist).toBeDefined();
	});

	it("Admin section items require admin", () => {
		const admin = MENU_SECTIONS.find((s) => s.title === "Admin");
		for (const item of admin?.items ?? []) {
			expect(item.requiresAdmin).toBe(true);
		}
	});
});

describe("getVisibleSections", () => {
	it("hides Admin section when not admin", () => {
		const visible = getVisibleSections(false);
		expect(visible.find((s) => s.title === "Admin")).toBeUndefined();
	});

	it("shows Admin section when admin", () => {
		const visible = getVisibleSections(true);
		expect(visible.find((s) => s.title === "Admin")).toBeDefined();
	});

	it("preserves non-admin items when not admin", () => {
		const visible = getVisibleSections(false);
		const browse = visible.find((s) => s.title === "Browse");
		const all = MENU_SECTIONS.find((s) => s.title === "Browse");
		expect(browse?.items.length).toBe(all?.items.length);
	});
});

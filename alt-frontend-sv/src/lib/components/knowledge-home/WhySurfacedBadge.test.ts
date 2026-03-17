import { describe, it, expect } from "vitest";
import { resolveWhyReason } from "./why-reason-map";

describe("WhySurfacedBadge reason mapping", () => {
	it("maps new_unread to 'New' with blue color", () => {
		const result = resolveWhyReason("new_unread");
		expect(result.label).toBe("New");
		expect(result.iconName).toBe("Sparkles");
		expect(result.colorClass).toContain("blue");
	});

	it("maps in_weekly_recap to 'In Recap' with purple color", () => {
		const result = resolveWhyReason("in_weekly_recap");
		expect(result.label).toBe("In Recap");
		expect(result.iconName).toBe("CalendarRange");
		expect(result.colorClass).toContain("purple");
	});

	it("maps tag_hotspot to 'Trending: {tag}' with green color", () => {
		const result = resolveWhyReason("tag_hotspot", "AI");
		expect(result.label).toBe("Trending: AI");
		expect(result.iconName).toBe("Tag");
		expect(result.colorClass).toContain("green");
	});

	it("maps tag_hotspot without tag to 'Trending'", () => {
		const result = resolveWhyReason("tag_hotspot");
		expect(result.label).toBe("Trending");
	});

	it("maps summary_completed to 'Summarized' with teal color", () => {
		const result = resolveWhyReason("summary_completed");
		expect(result.label).toBe("Summarized");
		expect(result.iconName).toBe("FileText");
		expect(result.colorClass).toContain("teal");
	});

	it("maps pulse_need_to_know to 'Need to Know' with orange color", () => {
		const result = resolveWhyReason("pulse_need_to_know");
		expect(result.label).toBe("Need to Know");
		expect(result.iconName).toBe("Activity");
		expect(result.colorClass).toContain("orange");
	});

	it("returns fallback for unknown code", () => {
		const result = resolveWhyReason("unknown_reason");
		expect(result.label).toBe("Info");
		expect(result.iconName).toBe("Info");
		expect(result.colorClass).toContain("gray");
	});
});

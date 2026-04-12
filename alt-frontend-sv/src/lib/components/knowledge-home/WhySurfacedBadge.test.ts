import { describe, expect, it } from "vitest";
import { resolveWhyReason } from "./why-reason-map";

describe("WhySurfacedBadge reason mapping (3-tier accents)", () => {
	it("maps new_unread to 'New' with accent-info", () => {
		const result = resolveWhyReason("new_unread");
		expect(result.label).toBe("New");
		expect(result.iconName).toBe("Sparkles");
		expect(result.colorClass).toContain("accent-info");
	});

	it("maps in_weekly_recap to 'In Recap' with accent-muted", () => {
		const result = resolveWhyReason("in_weekly_recap");
		expect(result.label).toBe("In Recap");
		expect(result.iconName).toBe("CalendarRange");
		expect(result.colorClass).toContain("accent-muted");
	});

	it("maps tag_hotspot with tag to 'Trending: {tag}' with accent-muted", () => {
		const result = resolveWhyReason("tag_hotspot", "AI");
		expect(result.label).toBe("Trending: AI");
		expect(result.iconName).toBe("Tag");
		expect(result.colorClass).toContain("accent-muted");
	});

	it("maps tag_hotspot without tag to 'Trending'", () => {
		const result = resolveWhyReason("tag_hotspot");
		expect(result.label).toBe("Trending");
	});

	it("maps summary_completed to 'Summarized' with accent-info", () => {
		const result = resolveWhyReason("summary_completed");
		expect(result.label).toBe("Summarized");
		expect(result.iconName).toBe("FileText");
		expect(result.colorClass).toContain("accent-info");
	});

	it("maps pulse_need_to_know to 'Need to Know' with accent-emphasis", () => {
		const result = resolveWhyReason("pulse_need_to_know");
		expect(result.label).toBe("Need to Know");
		expect(result.iconName).toBe("Activity");
		expect(result.colorClass).toContain("accent-emphasis");
	});

	it("returns fallback with accent-muted for unknown code", () => {
		const result = resolveWhyReason("unknown_reason");
		expect(result.label).toBe("Info");
		expect(result.iconName).toBe("Info");
		expect(result.colorClass).toContain("accent-muted");
	});
});

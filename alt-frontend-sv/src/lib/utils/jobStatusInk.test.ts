import { describe, expect, it } from "vitest";
import { statusToInk, statusToGlyph, statusToLabel } from "./jobStatusInk";

describe("statusToInk", () => {
	it("maps completed/succeeded to success ink", () => {
		expect(statusToInk("completed")).toBe("success");
		expect(statusToInk("succeeded")).toBe("success");
	});

	it("maps running to neutral ink (charcoal handles emphasis)", () => {
		expect(statusToInk("running")).toBe("neutral");
	});

	it("maps pending to muted ink", () => {
		expect(statusToInk("pending")).toBe("muted");
	});

	it("maps failed to error ink", () => {
		expect(statusToInk("failed")).toBe("error");
	});
});

describe("statusToGlyph", () => {
	it("returns ✓ for completed/succeeded", () => {
		expect(statusToGlyph("completed")).toBe("✓");
		expect(statusToGlyph("succeeded")).toBe("✓");
	});

	it("returns ● for running", () => {
		expect(statusToGlyph("running")).toBe("●");
	});

	it("returns ○ for pending", () => {
		expect(statusToGlyph("pending")).toBe("○");
	});

	it("returns ✗ for failed", () => {
		expect(statusToGlyph("failed")).toBe("✗");
	});
});

describe("statusToLabel", () => {
	it("returns visually-hidden labels in plain functional words", () => {
		expect(statusToLabel("completed")).toBe("Completed");
		expect(statusToLabel("succeeded")).toBe("Succeeded");
		expect(statusToLabel("running")).toBe("Running");
		expect(statusToLabel("pending")).toBe("Pending");
		expect(statusToLabel("failed")).toBe("Failed");
	});
});

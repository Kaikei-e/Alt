import { describe, expect, it } from "vitest";
import {
	ALT_PAPER_DEFAULTS,
	buildPalette,
	contrastRatio,
	relativeLuminance,
} from "./palette.js";

describe("palette", () => {
	it("computes WCAG contrast >= 3:1 against paper for every node ramp step", () => {
		const palette = buildPalette(
			ALT_PAPER_DEFAULTS.paper,
			ALT_PAPER_DEFAULTS.ink,
			ALT_PAPER_DEFAULTS.accent,
		);

		expect(palette.ramp).toHaveLength(5);
		for (const step of palette.ramp) {
			expect(contrastRatio(step, palette.paper)).toBeGreaterThanOrEqual(3.0);
		}
	});

	it("darkens the node ramp monotonically from lightest bucket to darkest bucket", () => {
		const palette = buildPalette(
			ALT_PAPER_DEFAULTS.paper,
			ALT_PAPER_DEFAULTS.ink,
			ALT_PAPER_DEFAULTS.accent,
		);

		const luminances = palette.ramp.map((c) => relativeLuminance(c));
		for (let i = 1; i < luminances.length; i++) {
			expect(luminances[i]).toBeLessThan(luminances[i - 1] as number);
		}
	});

	it("ensures accent has >= 3:1 contrast and labelInk has >= 4.5:1 contrast against paper", () => {
		const palette = buildPalette(
			ALT_PAPER_DEFAULTS.paper,
			ALT_PAPER_DEFAULTS.ink,
			ALT_PAPER_DEFAULTS.accent,
		);

		expect(contrastRatio(palette.accent, palette.paper)).toBeGreaterThanOrEqual(
			3.0,
		);
		expect(
			contrastRatio(palette.labelInk, palette.paper),
		).toBeGreaterThanOrEqual(4.5);
	});

	it("falls back to Alt-Paper token defaults when inputs are unparseable or gradients", () => {
		const palette = buildPalette(
			"linear-gradient(135deg, #000 0%, #fff 100%)",
			"not-a-color",
			"",
		);

		expect(palette.paper).toBe(ALT_PAPER_DEFAULTS.paper);
		expect(palette.labelInk).toBe(ALT_PAPER_DEFAULTS.ink);
		expect(palette.accent).toBe(ALT_PAPER_DEFAULTS.accent);
	});

	it("pins WCAG contrast ratio maths to known reference values and throws on invalid colours", () => {
		expect(contrastRatio("#000000", "#ffffff")).toBeCloseTo(21, 5);
		expect(contrastRatio("#ffffff", "#000000")).toBeCloseTo(21, 5);
		expect(contrastRatio("#faf9f7", "#faf9f7")).toBeCloseTo(1, 5);
		expect(contrastRatio("#2f4f4f", "#2f4f4f")).toBeCloseTo(1, 5);

		expect(() => relativeLuminance("not-a-color")).toThrow();
		expect(() => contrastRatio("not-a-color", "#faf9f7")).toThrow();
	});

	it("falls back to Alt-Paper defaults when resolved accent (< 3:1) or labelInk (< 4.5:1) fails contrast against paper", () => {
		// Light/neon tokens against light paper
		const lowContrastOnPaper = buildPalette("#faf9f7", "#999999", "#7fe5f0");
		expect(lowContrastOnPaper.labelInk).toBe(ALT_PAPER_DEFAULTS.ink);
		expect(lowContrastOnPaper.accent).toBe(ALT_PAPER_DEFAULTS.accent);
		expect(
			contrastRatio(lowContrastOnPaper.labelInk, lowContrastOnPaper.paper),
		).toBeGreaterThanOrEqual(4.5);
		expect(
			contrastRatio(lowContrastOnPaper.accent, lowContrastOnPaper.paper),
		).toBeGreaterThanOrEqual(3.0);

		// Dark surface tokens where dark ink fails against dark background
		const darkSurface = buildPalette("#0a0a0f", "#1a1a1a", "#2f4f4f");
		expect(
			contrastRatio(darkSurface.labelInk, darkSurface.paper),
		).toBeGreaterThanOrEqual(4.5);
		expect(
			contrastRatio(darkSurface.accent, darkSurface.paper),
		).toBeGreaterThanOrEqual(3.0);
		for (const step of darkSurface.ramp) {
			expect(contrastRatio(step, darkSurface.paper)).toBeGreaterThanOrEqual(
				3.0,
			);
		}
	});
});

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
});

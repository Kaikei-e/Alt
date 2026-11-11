import { describe, expect, it } from "vitest";
import {
	ALT_PAPER_DEFAULTS,
	buildPalette,
	clusterBaseInk,
	clusterInkForTag,
	contrastRatio,
	mixTowardPaper,
	relativeLuminance,
	TOPIC_INKS,
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

	it("maps clusterId 1..8 deterministically to the eight muted print inks and keeps every bucket >= 3:1 against #faf9f7", () => {
		const palette = buildPalette(
			ALT_PAPER_DEFAULTS.paper,
			ALT_PAPER_DEFAULTS.ink,
			ALT_PAPER_DEFAULTS.accent,
		);

		expect(TOPIC_INKS).toEqual([
			"#2b3a67",
			"#8c1d1d",
			"#2f6b3a",
			"#8a5a00",
			"#5b3a5e",
			"#2f4f4f",
			"#8b4a2b",
			"#3f5f7f",
		]);

		for (let clusterId = 1; clusterId <= 8; clusterId++) {
			const expectedBase = TOPIC_INKS[clusterId - 1] as string;
			expect(clusterBaseInk(clusterId, palette)).toBe(expectedBase);
			expect(clusterBaseInk(clusterId + 8, palette)).toBe(expectedBase);
			expect(
				contrastRatio(expectedBase, ALT_PAPER_DEFAULTS.paper),
			).toBeGreaterThanOrEqual(3.0);

			const lightest = clusterInkForTag(clusterId, 0, palette);
			const darkest = clusterInkForTag(clusterId, 4, palette);
			expect(darkest).toBe(expectedBase);
			expect(relativeLuminance(lightest)).toBeGreaterThan(
				relativeLuminance(darkest),
			);

			for (const bucket of [0, 1, 2, 3, 4] as const) {
				const variant = clusterInkForTag(clusterId, bucket, palette);
				expect(
					contrastRatio(variant, ALT_PAPER_DEFAULTS.paper),
				).toBeGreaterThanOrEqual(3.0);
			}
		}

		// clusterId 0 uses the existing neutral ink ramp
		for (const bucket of [0, 1, 2, 3, 4] as const) {
			expect(clusterInkForTag(0, bucket, palette)).toBe(palette.ramp[bucket]);
		}

		// 75% mix toward paper for de-emphasised nodes
		const faded = mixTowardPaper("#2b3a67", ALT_PAPER_DEFAULTS.paper, 0.75);
		expect(relativeLuminance(faded)).toBeGreaterThan(
			relativeLuminance("#2b3a67"),
		);
		expect(relativeLuminance(faded)).toBeLessThan(
			relativeLuminance(ALT_PAPER_DEFAULTS.paper),
		);
	});
});

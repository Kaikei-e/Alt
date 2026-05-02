import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

/**
 * Structural guard for OodaPipeline.
 *
 * The pipeline ribbon expresses the OODA cycle in 3D space (canonical contract
 * §12). The contract this test pins:
 *   - The masthead's stage indicator is a perspective ribbon, not a flat
 *     `kicker-row`. translateZ + saturation carry the depth; glyphs stay flat.
 *   - The active stage (data-depth=0) returns to Z=0 with the strongest
 *     saturation. Subsequent stages recede along the cycle and desaturate.
 *   - The reduced-motion media query collapses every kicker's transform so
 *     the ribbon flattens to a row, with active=charcoal / others=ash.
 *   - There is a wrap arrow (↻) closing the cycle Act → Observe so the
 *     loop reads as continuous, not as a one-way pipeline.
 */

const source = readFileSync(
	fileURLToPath(new URL("./OodaPipeline.svelte", import.meta.url)),
	"utf-8",
);

describe("OodaPipeline source guards", () => {
	it("renders all four OODA stages with stable labels", () => {
		expect(source).toMatch(/observe[\s\S]*?Observe/);
		expect(source).toMatch(/orient[\s\S]*?Orient/);
		expect(source).toMatch(/decide[\s\S]*?Decide/);
		expect(source).toMatch(/act[\s\S]*?Act/);
	});

	it("establishes a local 3D context so kicker depths render against a shared vanishing point", () => {
		expect(source).toMatch(/perspective:\s*\d+px/);
		expect(source).toMatch(/transform-style:\s*preserve-3d/);
	});

	it("maps each kicker depth band to a distinct translateZ", () => {
		expect(source).toMatch(/\[data-depth="0"\][\s\S]*?translateZ\(0/);
		expect(source).toMatch(/\[data-depth="1"\][\s\S]*?translateZ\(-\d+px\)/);
		expect(source).toMatch(/\[data-depth="2"\][\s\S]*?translateZ\(-\d+px\)/);
		expect(source).toMatch(/\[data-depth="3"\][\s\S]*?translateZ\(-\d+px\)/);
	});

	it("closes the loop visually with a wrap arrow after Act", () => {
		// The cycle's continuity is the whole point — a flat row of kickers
		// with `→` between them does not encode it. The wrap arrow `↻` is
		// what tells the eye that Act flows back to Observe.
		expect(source).toMatch(/arrow--wrap/);
		expect(source).toMatch(/↻/);
	});

	it("can act as the OODA stage controller when a transition callback is provided", () => {
		expect(source).toMatch(/onStageSelect/);
		expect(source).toMatch(/<button/);
		expect(source).toMatch(/onclick=\{\(\) => selectStage\(stage\.name\)\}/);
		expect(source).toMatch(/aria-current=\{depth === 0 \? "step" : undefined\}/);
	});

	it("keeps a reduced-motion fallback that flattens the ribbon", () => {
		expect(source).toMatch(/prefers-reduced-motion[\s\S]*?reduce/);
		// Reduced motion must remove translateZ from every kicker bucket.
		expect(source).toMatch(
			/@media\s*\(prefers-reduced-motion:\s*reduce\)[\s\S]*?transform:\s*none/,
		);
	});

	it("does not introduce drop-shadows (Alt-Paper rule)", () => {
		// The pipeline rides Alt-Paper's "no shadows" rule — depth comes from
		// translateZ + saturation only. A regression that adds box-shadow or
		// drop-shadow here would break the design system's vocabulary.
		expect(source).not.toMatch(/box-shadow:/);
		expect(source).not.toMatch(/drop-shadow\(/);
	});
});

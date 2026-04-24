import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

/**
 * Source-level guards for `loop-depth.css` — ADR-000831 §11-12 (spatial render
 * contract). These tests pin the canonical plane hierarchy so a future style
 * sweep cannot silently drop the Reduced Motion fallback or re-introduce
 * motion primitives the contract forbids.
 */

const cssSource = readFileSync(
	fileURLToPath(new URL("./loop-depth.css", import.meta.url)),
	"utf-8",
);

describe("loop-depth.css — plane hierarchy contract", () => {
	it("defines the three canonical planes", () => {
		expect(cssSource).toMatch(/data-plane="foreground"/);
		expect(cssSource).toMatch(/data-plane="mid-context"/);
		expect(cssSource).toMatch(/data-plane="deep-focus"/);
	});

	it("applies a translateZ gradient so mid-context sits between foreground and deep-focus", () => {
		// Foreground: translateZ(0). Mid and deep use negative Z so they recede.
		expect(cssSource).toMatch(/foreground[\s\S]*?translateZ\(0\)/);
		expect(cssSource).toMatch(/mid-context[\s\S]*?translateZ\(-\d+px\)/);
		expect(cssSource).toMatch(/deep-focus[\s\S]*?translateZ\(-\d+px\)/);
	});

	it("sets perspective on the loop-plane-root so children share a vanishing point", () => {
		expect(cssSource).toMatch(/\.loop-plane-root[\s\S]*?perspective:\s*\d+px/);
	});

	it("reduces saturation as the plane recedes (visual depth cue beyond translateZ)", () => {
		expect(cssSource).toMatch(/foreground[\s\S]*?saturate\(1\.0[0-9]\)/);
		expect(cssSource).toMatch(/mid-context[\s\S]*?saturate\(0\.9[0-9]\)/);
		expect(cssSource).toMatch(/deep-focus[\s\S]*?saturate\(0\.[78][0-9]\)/);
	});
});

describe("loop-depth.css — Reduced Motion fallback", () => {
	it("guards translateZ / scale with a prefers-reduced-motion media query", () => {
		// The media block must disable translateZ / perspective on mid and deep.
		expect(cssSource).toMatch(
			/@media \(prefers-reduced-motion: reduce\)[\s\S]*?loop-plane-root[\s\S]*?perspective:\s*none/,
		);
		expect(cssSource).toMatch(
			/@media \(prefers-reduced-motion: reduce\)[\s\S]*?mid-context[\s\S]*?transform:\s*none/,
		);
		expect(cssSource).toMatch(
			/@media \(prefers-reduced-motion: reduce\)[\s\S]*?deep-focus[\s\S]*?transform:\s*none/,
		);
	});

	it("still keeps opacity transitions so planes read as layered for motion-sensitive users", () => {
		// Reduced Motion preserves opacity-only transition per contract §12.5.
		const reducedMotionBlock = cssSource.match(
			/@media \(prefers-reduced-motion: reduce\)[\s\S]*?}\s*}\s*$/m,
		);
		expect(reducedMotionBlock).not.toBeNull();
		expect(reducedMotionBlock?.[0]).toMatch(/opacity\s+0\.\d+s/);
	});
});

import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

/**
 * Regression gate for the SSR preload-warning fix.
 *
 * SvelteKit emits the full chunk graph through the `Link:` HTTP header as
 * `rel="preload"; as="style"` (CSS) + `rel="modulepreload"` (JS). For CSS
 * chunks that also appear in the HTML `<head>` as `<link rel="stylesheet">`,
 * Chrome flags the preload side as "preloaded using link preload but not used
 * within a few seconds from the window's load event". The DebugBear analysis
 * matches: "duplicate resource loading" — preload + stylesheet on the same
 * URL trips the heuristic.
 *
 * `kit.inlineStyleThreshold` is SvelteKit's first-party knob for this: any CSS
 * chunk smaller than the threshold (in UTF-16 code units ≈ bytes for ASCII)
 * is merged into a `<style>` block and the preload directive disappears for
 * it. The big root-layout CSS (`0.*.css` ≈ 85 KB) stays external for cache
 * efficiency, so one warning may persist until upstream Issue #8549 lands a
 * `modulepreload: 'tag' | 'header'` switch.
 *
 * Pin the threshold to a sane window so a future config refactor cannot
 * silently regress to 0 (the SvelteKit default) without this test screaming.
 */

const configSource = readFileSync(
	fileURLToPath(new URL("../../svelte.config.js", import.meta.url)),
	"utf-8",
);

describe("svelte.config.js — preload warning regression gate", () => {
	it("declares kit.inlineStyleThreshold", () => {
		expect(configSource).toMatch(/inlineStyleThreshold\s*:\s*\d+/);
	});

	it("sets inlineStyleThreshold within the documented sweet spot (4096..8192 bytes)", () => {
		// 4096 is small enough to catch every observed route-specific CSS
		// chunk (1.9 KB / 4.5 KB / etc.) but leaves the 85 KB root layout
		// external. 8192 caps HTML inflation so each route response stays
		// cache-friendly.
		const match = configSource.match(/inlineStyleThreshold\s*:\s*(\d+)/);
		expect(match).not.toBeNull();
		const value = Number(match?.[1]);
		expect(value).toBeGreaterThanOrEqual(4096);
		expect(value).toBeLessThanOrEqual(8192);
	});

	it("keeps the default modulepreload strategy implicit (no override)", () => {
		// SvelteKit's `'modulepreload'` default is officially best on Chrome
		// / Firefox 115+ / Safari 17+. `'preload-mjs'` / `'preload-js'` are
		// only useful for legacy iOS Safari and would double the warning
		// surface area. If a future PR feels the need to override, the
		// reason has to be documented in this comment first.
		expect(configSource).not.toMatch(/preloadStrategy\s*:/);
	});
});

import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

/**
 * Structural guard for LoopEntryTile.
 *
 * Complements LoopEntryTile.svelte.spec.ts (browser-mode rendering, runs under
 * test:client). These assertions pin the *contract* of the tile source so
 * changes cannot silently drop single-emission, a11y, or PR-L2 CTA wiring.
 * Runtime interaction is covered by the Playwright spec
 * tests/e2e/mobile/loop-transition.spec.ts.
 *
 * Invariants pinned here:
 *   §8 (ADR-000831)  — single-emission: must not import knowledge_home.
 *   §12               — reduced-motion dissolve branch still present.
 *   Invariant 32     — ask intent is filtered on the UI side.
 *   PR-L2 plan       — Open / Save / Snooze / Dismiss CTAs are named functional words.
 */

const tileSource = readFileSync(
	fileURLToPath(new URL("./LoopEntryTile.svelte", import.meta.url)),
	"utf-8",
);

describe("LoopEntryTile source guards", () => {
	it("accepts the PR-L2 props via $props() destructuring", () => {
		expect(tileSource).toMatch(/onTransition/);
		expect(tileSource).toMatch(/onDismiss/);
		expect(tileSource).toMatch(/canTransition/);
		expect(tileSource).toMatch(/isInFlight/);
		expect(tileSource).toMatch(/resolveSourceUrl/);
	});

	it("marks the tile root as an expandable button for keyboard and AT users", () => {
		expect(tileSource).toMatch(/role=["']button["']/);
		expect(tileSource).toMatch(/aria-expanded/);
	});

	it("exposes data-entry-key so the page-level IntersectionObserver can track it", () => {
		expect(tileSource).toMatch(/data-entry-key=\{entry\.entryKey\}/);
	});

	it("filters out ask-intent CTAs until PR-L3 wires the Augur handshake", () => {
		expect(tileSource).toMatch(/intent\s*!==\s*["']ask["']/);
	});

	it("renders the four PR-L2 CTAs as uppercase functional-word buttons", () => {
		for (const label of ["Open", "Save", "Snooze", "Dismiss"]) {
			expect(tileSource).toContain(label);
		}
	});

	it("never imports knowledge_home (ADR-000831 §8 single-emission)", () => {
		expect(tileSource).not.toMatch(/\$lib\/connect\/knowledge_home/);
		expect(tileSource).not.toMatch(/trackHomeAction/);
	});

	it("keeps the reduced-motion media query that downgrades to dissolve only", () => {
		expect(tileSource).toMatch(/prefers-reduced-motion/);
	});

	it("opens external URLs with noopener and a _blank target", () => {
		expect(tileSource).toMatch(/_blank/);
		expect(tileSource).toMatch(/noopener/);
	});

	it("still routes why_primary text through escaped interpolation (no @html)", () => {
		expect(tileSource).not.toMatch(/\{@html/);
		expect(tileSource).toMatch(/\{entry\.whyPrimary\.text\}/);
	});
});

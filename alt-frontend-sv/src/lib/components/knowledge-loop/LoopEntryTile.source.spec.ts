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

	it("does not await transition acceptance before handling the Open CTA", () => {
		expect(tileSource).toMatch(/function openHref/);
		expect(tileSource).toMatch(
			/void onTransition\(entry\.entryKey, to, ["']user_tap["'], metadata\)/,
		);
		expect(tileSource).not.toMatch(/const result = await onTransition/);
	});

	it("still routes why_primary text through escaped interpolation (no @html)", () => {
		expect(tileSource).not.toMatch(/\{@html/);
		expect(tileSource).toMatch(/\{entry\.whyPrimary\.text\}/);
	});

	it("maps each OODA stage to a distinct translateZ band (canonical contract §12)", () => {
		// Each OODA stage gets its own Z position inside the foreground plane.
		// observe sits in front (Z=0), act recedes furthest. The cycle then
		// wraps via `act → observe` on transitionTo. Reduced motion strips Z
		// (covered by the @media query test above).
		expect(tileSource).toMatch(
			/\.entry\[data-stage="observe"\][\s\S]*?translateZ\(0/,
		);
		expect(tileSource).toMatch(
			/\.entry\[data-stage="orient"\][\s\S]*?translateZ\(-\d+px\)/,
		);
		expect(tileSource).toMatch(
			/\.entry\[data-stage="decide"\][\s\S]*?translateZ\(-\d+px\)/,
		);
		expect(tileSource).toMatch(
			/\.entry\[data-stage="act"\][\s\S]*?translateZ\(-\d+px\)/,
		);
	});

	it("transitions transform smoothly so a stage change is animated", () => {
		expect(tileSource).toMatch(/transition:[\s\S]*?transform\s+\d+ms/);
	});

	// --- Recap first-class CTA (Stream 2C) -----------------------------------
	// The projector seeds entry.actTargets with `{targetType: "recap", route:
	// "/recap/topic/<id>"}` when SurfaceScoreInputs.RecapTopicSnapshotID is
	// non-empty. The tile renders an "Open Recap" CTA that links to that
	// route. The route must be validated as server-relative + scheme-free so
	// a future resolver bug cannot smuggle a `javascript:` URL through.

	it("renders an Open Recap CTA when entry.actTargets contains a recap target", () => {
		// The CTA is rendered as a real anchor (semantic link, ctrl+click,
		// keyboard) and its label is the functional word "Open Recap" — Alt-
		// Paper keeps metaphor in the visual layer; CTA text stays functional.
		expect(tileSource).toContain("Open Recap");
		expect(tileSource).toMatch(
			/actTargets[\s\S]*?targetType\s*===\s*["']recap["']/,
		);
	});

	it("guards the recap route against javascript: schemes and absolute URLs", () => {
		// Defense in depth: the projector already rejects non-UUID snapshot
		// ids, but the FE refuses to render a CTA whose route contains ":"
		// or doesn't start with "/". Open-redirect / scheme-injection
		// (OWASP A01/A05) cannot land even if upstream regresses.
		expect(tileSource).toMatch(/startsWith\(["']\/["']\)/);
		expect(tileSource).toMatch(/\.includes\(["']:["']\)/);
	});

	// CWE-601 / OWASP A01 (open-redirect) defence-in-depth gap: a route
	// like `//evil.com/x` passes `startsWith("/")` and contains no `:`, so
	// the original two-line allowlist let it render as a protocol-relative
	// anchor. Browsers resolve `<a href="//evil.com/x">` to the origin's
	// scheme + evil.com — full open-redirect. The component must explicitly
	// reject `//` and the backslash-normalised `/\\` shape too.
	it("rejects protocol-relative URLs (CWE-601 open-redirect bypass)", () => {
		expect(tileSource).toMatch(/startsWith\(["']\/\/["']\)/);
		expect(tileSource).toMatch(/startsWith\(["']\/\\\\["']\)/);
	});
});

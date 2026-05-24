import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

/**
 * Source-hygiene + structural assertions for WhyTypography.
 *
 * WhyTypography is the shared Newspaper-Style display primitive for a
 * single WhyPayload (ADR-000908 §Δ4: confidence ladder + counter-evidence
 * land Why first-class). The grep-based assertions co-locate the
 * "must / must-not" with the component so a regression that strips
 * the confidence ladder, breaks escape-discipline, or silently drops the
 * counter-evidence disclosure fails CI without spinning up the renderer.
 */

const source = readFileSync(
	fileURLToPath(new URL("./WhyTypography.svelte", import.meta.url)),
	"utf-8",
);

describe("WhyTypography source hygiene", () => {
	it("never uses {@html} to render user-controlled text", () => {
		// WhyPayload.text and counter-evidence labels arrive from the proto
		// boundary as plain text. They MUST flow through Svelte's escaping
		// interpolation. Canonical contract §27 / F-009.
		expect(source).not.toMatch(/\{@html\s/);
	});

	it("renders the why narrative via escaped interpolation", () => {
		expect(source).toMatch(/\{text\}/);
	});

	it("declares the four confidence ladder tiers as an exhaustive list", () => {
		// Pinning the four tiers in the source guards against a refactor
		// that drops one (a missing tier means the bar that should fill
		// silently stays empty).
		for (const tier of ["SPECULATION", "PATTERN", "EVIDENCE", "VERIFIED"]) {
			expect(source).toMatch(new RegExp(`['"]${tier}['"]`));
		}
	});

	it("suppresses the ladder when confidenceLadder is UNSPECIFIED or missing", () => {
		// The UI must hide the indicator rather than show four empty bars.
		// Easiest enforcement is a guard against rendering when the tier
		// resolves to a 0-of-4 fill.
		expect(source).toMatch(
			/UNSPECIFIED|ladderStep\s*[<>=!]==?\s*0|ladderStep\s*===?\s*null/,
		);
	});

	it("declares the seven WhyKind labels used by the Loop variant", () => {
		// The kind label is the uppercase Newspaper Style banner above the
		// narrative. All seven enum values must have an explicit label so a
		// new kind never falls back to the empty string by accident.
		for (const kind of [
			"source_why",
			"pattern_why",
			"recall_why",
			"change_why",
			"topic_affinity_why",
			"tag_trending_why",
			"unfinished_continue_why",
		]) {
			expect(source).toMatch(new RegExp(`['"]${kind}['"]`));
		}
	});

	it("uses a disclosure-style toggle for counter-evidence", () => {
		// ADR-000908 §Δ4: counter-evidence (objections) is folded by default
		// so it doesn't crowd the primary why. The component must render an
		// actual <button> with aria-expanded so screen readers announce the
		// state, not a div with click-only behaviour.
		expect(source).toMatch(/aria-expanded/);
		expect(source).toMatch(/<button/);
	});

	it("respects prefers-reduced-motion in the disclosure transition", () => {
		// Newspaper Style invariant + accessibility: motion is opt-out.
		expect(source).toMatch(/prefers-reduced-motion/);
	});

	it("uses Alt design tokens (sepia ladder + font tokens) rather than hard-coded colours", () => {
		// The CSS must inherit from the Alt token vocabulary so a theme
		// change flows through without component edits.
		expect(source).toMatch(/var\(--font-display/);
		expect(source).toMatch(/var\(--font-mono/);
		expect(source).toMatch(/var\(--alt-charcoal/);
	});
});

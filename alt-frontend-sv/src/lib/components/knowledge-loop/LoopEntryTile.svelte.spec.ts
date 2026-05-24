import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

/**
 * Guard test: the tile MUST render user text with `{text}` semantics (escaped).
 * Per ADR-000831 and canonical contract §27 / F-009, WhyPayload.text is plain text
 * and MUST NOT flow through `{@html}`. A regex grep is the simplest enforcement
 * we can co-locate with the component.
 */

const tileSource = readFileSync(
	fileURLToPath(new URL("./LoopEntryTile.svelte", import.meta.url)),
	"utf-8",
);

describe("LoopEntryTile source hygiene", () => {
	it("never uses {@html} to render user text", () => {
		expect(tileSource).not.toMatch(/\{@html\s/);
	});

	it("delegates the why payload to the WhyTypography primitive", () => {
		// ADR-000911 Phase B: the inline why-text + evidence list moved into
		// `lib/components/why/WhyTypography.svelte` so Loop and Home share a
		// single Newspaper-Style presentation surface. The tile MUST pass the
		// payload through props rather than reintroducing an inline renderer
		// — duplicating the escape discipline in two places is exactly what
		// invited the F-009 regressions canonical contract §27 warns about.
		expect(tileSource).toMatch(/import\s+WhyTypography/);
		expect(tileSource).toMatch(
			/<WhyTypography[\s\S]*?text=\{entry\.whyPrimary\.text\}/,
		);
		expect(tileSource).toMatch(
			/counterEvidenceRefs=\{entry\.whyPrimary\.counterEvidenceRefs\}/,
		);
	});

	it("emits an aria-label carrying the loop priority so assistive tech can read it", () => {
		expect(tileSource).toMatch(/aria-label=\{ariaDescription\}/);
		expect(tileSource).toMatch(/Priority:\s*\$\{priorityLabel\}/);
	});

	it("exposes the ADR-000914 'I got this' CTA only when onInternalize is wired", () => {
		// The CTA must (a) be conditional on the onInternalize callback so
		// surfaces without graduation semantics (e.g. recap-only embeds)
		// stay clean, (b) carry an aria-label that names the destination
		// state, and (c) call onInternalize with the entry payload so the
		// caller can build the canonical TRANSITION_TRIGGER_INTERNALIZE
		// transition without re-fetching the row.
		expect(tileSource).toMatch(/\{#if onInternalize\}/);
		expect(tileSource).toMatch(
			/aria-label="Mark as internalized; remove from Loop"/,
		);
		expect(tileSource).toMatch(/onInternalize\(entry\)/);
	});

	it("respects prefers-reduced-motion on the internalize CTA transition", () => {
		// Newspaper Style invariant + a11y: motion is opt-out everywhere.
		// The CTA's color transition must collapse under reduced-motion so
		// the click feedback stays static for users who request it.
		expect(tileSource).toMatch(
			/@media\s*\(\s*prefers-reduced-motion:\s*reduce\s*\)[\s\S]*?\.cta--internalize/,
		);
	});
});

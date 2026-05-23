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
});

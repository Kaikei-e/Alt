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

	it("renders the why text via escaping Svelte interpolation", () => {
		expect(tileSource).toMatch(/\{entry\.whyPrimary\.text\}/);
	});

	it("emits an aria-label carrying the loop priority so assistive tech can read it", () => {
		expect(tileSource).toMatch(/aria-label=\{ariaDescription\}/);
		expect(tileSource).toMatch(/Priority:\s*\$\{priorityLabel\}/);
	});
});

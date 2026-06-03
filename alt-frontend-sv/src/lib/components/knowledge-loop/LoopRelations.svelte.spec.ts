import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

/**
 * ADR-000937: the relation-set is the always-present Orient surface, rendered
 * by this shared component for both the active-entry workspace and the
 * secondary LoopEntryTile. Source-hygiene guards mirror the LoopEntryTile spec
 * convention.
 */

const source = readFileSync(
	fileURLToPath(new URL("./LoopRelations.svelte", import.meta.url)),
	"utf-8",
);

describe("LoopRelations source hygiene", () => {
	it("renders each relation as a chip exposing kind + state data attributes", () => {
		expect(source).toMatch(/data-testid="loop-relation"/);
		expect(source).toMatch(/data-relation-kind=\{relation\.kind\}/);
		expect(source).toMatch(/data-relation-state=\{relation\.state\}/);
	});

	it("never uses {@html} to render the relation why text", () => {
		// Relation why_text is plain text (same discipline as WhyPayload.text).
		expect(source).not.toMatch(/\{@html\s/);
		expect(source).toMatch(/\{relation\.whyText\}/);
	});

	it("encodes the relation State in the chip class for the return-diff cue", () => {
		// open → advancing → advanced changes the left-rule colour so the user
		// perceives the loop closing when an acted relation returns.
		expect(source).toMatch(/relation--\{relation\.state\}/);
		expect(source).toMatch(/\.relation--advancing/);
		expect(source).toMatch(/\.relation--advanced/);
	});
});

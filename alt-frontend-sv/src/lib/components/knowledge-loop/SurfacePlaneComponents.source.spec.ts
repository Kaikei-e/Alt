import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

/**
 * Source-level guards for the PR-L8 Surface plane components.
 *
 * Rationale: these components render user-facing text that originates from
 * append-only `knowledge_events` payloads. ADR-000831 / canonical contract §27
 * pin WhyPayload.text to plain text — `{@html}` is forbidden, and every
 * non-foreground entry must carry an `aria-description` derived from
 * LoopPriority so screen readers get the urgency signal independent of depth
 * rendering (contract §13).
 *
 * A regex-level check co-located with the component is the cheapest way to
 * prevent these invariants from regressing silently during a component rewrite.
 */

const componentSources = {
	ContinueStream: readFileSync(
		fileURLToPath(new URL("./ContinueStream.svelte", import.meta.url)),
		"utf-8",
	),
	ChangedDiffCard: readFileSync(
		fileURLToPath(new URL("./ChangedDiffCard.svelte", import.meta.url)),
		"utf-8",
	),
	ReviewDock: readFileSync(
		fileURLToPath(new URL("./ReviewDock.svelte", import.meta.url)),
		"utf-8",
	),
};

describe.each(
	Object.entries(componentSources),
)("%s source hygiene", (name, source) => {
	it(`${name} never renders user text via {@html} (XSS guard)`, () => {
		expect(source).not.toMatch(/\{@html\s/);
	});

	it(`${name} references whyPrimary.text via escaping Svelte interpolation`, () => {
		// Each plane surfaces the entry's why_text directly; ChangedDiffCard
		// falls back to change_summary.summary when available.
		expect(source).toMatch(/entry\.whyPrimary\.text/);
	});

	it(`${name} exposes an accessible priority label from loopPriority`, () => {
		// aria-label (over aria-description) carries the single-line urgency cue
		// for screen readers; aria-description lacks cross-browser typing support
		// in Svelte 5 HTMLProps and lands as a hard type error. The aria-label
		// expression must reach loopPriorityAriaLabel[entry.loopPriority] —
		// either inline or via a helper that returns it (e.g. ChangedDiffCard's
		// ariaSummary, which augments the priority with diff counts).
		expect(source).toMatch(/aria-label=/);
		expect(source).toMatch(/loopPriorityAriaLabel\[entry\.loopPriority\]/);
	});

	it(`${name} exposes a stable data-testid for E2E hooks`, () => {
		// Playwright E2E (tests/e2e) pins interaction targets by testid.
		expect(source).toMatch(/data-testid="loop-/);
	});
});

describe("ContinueStream carries a freshness stamp and resume callback", () => {
	const source = componentSources.ContinueStream;
	it("renders a monospace stamp computed from freshness_at", () => {
		expect(source).toMatch(/formatFreshness\(entry\.freshnessAt\)/);
	});
	it("wires the resume callback for transition handoff", () => {
		expect(source).toMatch(/onResume\?\.\(entry\)/);
	});
});

describe("ChangedDiffCard encodes the THEN / NOW diptych", () => {
	const source = componentSources.ChangedDiffCard;
	it("emits both Then and Now kickers so the layout is visually explicit", () => {
		expect(source).toMatch(/>\s*Then\s*</);
		expect(source).toMatch(/>\s*Now\s*</);
	});
	it("falls back from change_summary to the supersede pointer", () => {
		// Guard against silent regressions where the card renders an empty
		// left column when change_summary is missing.
		expect(source).toMatch(/entry\.supersededByEntryKey/);
	});
	it("wires the confirm callback so Act transitions can emit KnowledgeLoopActed", () => {
		expect(source).toMatch(/onConfirm\?\.\(entry\)/);
	});
});

describe("ReviewDock keeps the low-density dot rhythm", () => {
	const source = componentSources.ReviewDock;
	it("renders a dotted border so the plane reads as peripheral", () => {
		expect(source).toMatch(/dotted/);
	});
	it("wires the open callback for deep-focus entry", () => {
		expect(source).toMatch(/onOpen\?\.\(entry\)/);
	});
});

describe("Mid-context saturation respects Reduced Motion", () => {
	// LoopSurfacePlane carries the filter; the plane components themselves
	// must not reintroduce heavy transitions that bypass the reduced-motion
	// opt-out. Grep for translateZ / parallax primitives and fail if any
	// of the three components introduce them.
	it.each(
		Object.entries(componentSources),
	)("%s avoids translateZ / parallax primitives", (_name, source) => {
		expect(source).not.toMatch(/translateZ\(/);
		expect(source).not.toMatch(/perspective\(/);
	});
});

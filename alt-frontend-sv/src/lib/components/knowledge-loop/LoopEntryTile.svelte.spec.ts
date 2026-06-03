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

	it("dispatches Revisit as a same-stage intent_signal (ADR-000914)", () => {
		// Revisit must not force a backward orient transition that the
		// canonical transition policy rejects from decide / act. The CTA
		// fires with the user's current stage and the canonical
		// `intent_signal` same-stage trigger so the projector records the
		// re-engagement without flipping data-stage.
		expect(tileSource).toMatch(/SAME_STAGE_INTENT_TRIGGER/);
		expect(tileSource).toMatch(
			/SAME_STAGE_INTENT_TRIGGER[\s\S]*?\[\s*"revisit"\s*,\s*"intent_signal"\s*\]/,
		);
		expect(tileSource).toMatch(
			/onTransition\(\s*entry\.entryKey\s*,\s*effectiveStage\s*,\s*sameStageTrigger\s*,/,
		);
	});

	it("keeps same-stage CTAs enabled when onTransition is wired", () => {
		// Without this branch the disabled computation falls through to the
		// cross-stage `isAllowed(toStage)` check and Revisit is silently
		// rejected from any stage past observe (the bug logged 2026-05).
		expect(tileSource).toMatch(/isSameStageCta/);
		expect(tileSource).toMatch(
			/isSameStageCta[\s\S]*?inFlight\s*\|\|\s*!onTransition/,
		);
	});

	it("recovers article URL from entry_key when act_targets is empty", () => {
		// Defense-in-depth fallback for the Pillar 2 act_targets fix. The
		// projector now keeps the article target stable across augur events,
		// but a future regression must not leave Open Article silently dead;
		// the tile recovers the destination from the canonical
		// `entry:<article-id>` natural key. Restricted to UUID-shaped tails so
		// a malformed key cannot smuggle a route.
		expect(tileSource).toMatch(/articleFromEntryKey/);
		expect(tileSource).toMatch(/entryKey\.startsWith\("entry:"\)/);
		expect(tileSource).toMatch(
			/\/\^\[0-9a-fA-F\]\{8\}-\[0-9a-fA-F\]\{4\}-\[1-5\]/,
		);
	});
});

describe("LoopEntryTile relation-set (ADR-000937)", () => {
	it("delegates the relation-set to the shared LoopRelations component", () => {
		// The chip markup lives in LoopRelations so the active-entry workspace
		// and the secondary tiles render the relation-set identically. The tile
		// only threads its entry's relations through.
		expect(tileSource).toMatch(/import\s+LoopRelations/);
		expect(tileSource).toMatch(/<LoopRelations\s+\{relations\}\s*\/>/);
	});

	it("renders the relation-set as the always-present Orient surface (before the expand block)", () => {
		// Orient is a surface, not a stage you advance into: the relations must
		// render without expanding the tile. Enforce by source order — the
		// LoopRelations element precedes the `{#if expanded}` block.
		const relationsIdx = tileSource.indexOf("<LoopRelations");
		const expandIdx = tileSource.indexOf("{#if expanded}");
		expect(relationsIdx).toBeGreaterThan(-1);
		expect(expandIdx).toBeGreaterThan(-1);
		expect(relationsIdx).toBeLessThan(expandIdx);
	});
});

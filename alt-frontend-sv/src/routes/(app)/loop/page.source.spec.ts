import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

/**
 * Structural guard for /loop/+page.svelte.
 *
 * Pins the page wiring: instantiates `useKnowledgeLoop`, forwards hook
 * actions (onTransition / onDismiss / canTransition / isInFlight) into each
 * tile, and — post Auto-OODA suppression (Knowledge Loop 体験回復 plan,
 * Pillar 1) — does NOT attach an IntersectionObserver-driven `observeTiles`
 * action. Passive viewing must not advance OODA stage.
 */

const pageSource = readFileSync(
	fileURLToPath(new URL("./+page.svelte", import.meta.url)),
	"utf-8",
);

describe("/loop/+page.svelte wiring guards", () => {
	it("instantiates useKnowledgeLoop with the loader's initial payload", () => {
		expect(pageSource).toMatch(/useKnowledgeLoop\s*\(/);
		expect(pageSource).toMatch(
			/from\s+["']\$lib\/hooks\/useKnowledgeLoop\.svelte["']/,
		);
	});

	it("does NOT attach observeTiles or any dwell-driven observer", () => {
		// Auto-OODA suppression: passive viewing must not fire transitions.
		expect(pageSource).not.toMatch(/use:observeTiles/);
		expect(pageSource).not.toMatch(/onObserve\b/);
		expect(pageSource).not.toMatch(/observe-tiles/);
		expect(pageSource).not.toMatch(/loop\.observe\(/);
	});

	it("forwards the transition / dismiss / gating handlers to each tile", () => {
		expect(pageSource).toMatch(/onTransition/);
		expect(pageSource).toMatch(/onDismiss/);
		expect(pageSource).toMatch(/canTransition/);
		expect(pageSource).toMatch(/isInFlight/);
	});

	it("renders a retry-friendly banner copy for transient Connect codes", () => {
		// 2026-05-28 deploy-gap incident: a 5-second knowledge-sovereign restart
		// surfaced as "Loop unavailable / [internal]" because the FE printed
		// data.error verbatim. The banner must distinguish transient codes
		// ("unavailable" / "deadline_exceeded" / "canceled" — now propagated
		// from alt-backend) from genuine internal failures so the user knows a
		// refresh will resolve it.
		expect(pageSource).toMatch(/data\.error\s*===?\s*["']unavailable["']/);
		expect(pageSource).toMatch(/briefly unavailable/);
	});

	it("excludes the workspace spotlight entry from the NOW plane bench when foreground has siblings", () => {
		// When foreground holds 2+ entries, the .ooda-workspace section already
		// renders activeEntry (= foreground[0]) with its own action buttons; the
		// NOW plane MUST NOT render that same entry again as a LoopEntryTile or
		// the same article shows two stacked sets of REVISIT/ASK/SNOOZE controls
		// (regression reported 2026-05-27 via screenshot). When foreground holds
		// exactly one entry the tile stays rendered — the bench would otherwise
		// be empty and the e2e suite would lose its single-entry target.
		expect(pageSource).toMatch(
			/foreground\.length\s*>\s*1\s*\?\s*foreground\.filter\([\s\S]*?activeEntry\?\.entryKey[\s\S]*?\)\s*:\s*foreground/,
		);
	});

	it("derives bucket planes from hook-owned bucketEntries", () => {
		// ADR-000908 §Δ3 added a filter clause that excludes `internalized`
		// entries from foreground / bucket surfaces. The source must still
		// derive from loop.bucketEntries and must never re-read the raw
		// data.loop snapshot here (that path mixes ungated SSR state with
		// optimistic Runes state).
		expect(pageSource).toMatch(
			/const bucketEntries = \$derived\([\s\S]*loop\.bucketEntries/,
		);
		expect(pageSource).not.toMatch(
			/const bucketEntries = \$derived\(data\.loop\?\.bucketEntries/,
		);
	});

	// ADR-000908 §Δ3: foreground and bucket planes filter out internalized
	// entries so the "I got this" graduation removes the row from every
	// surface (only the MacroByline counter still references it).
	it("filters internalized entries out of foreground and bucket planes", () => {
		expect(pageSource).toMatch(
			/foreground = \$derived\([\s\S]*dismissState !== "internalized"/,
		);
		expect(pageSource).toMatch(
			/const bucketEntries = \$derived\([\s\S]*dismissState !== "internalized"/,
		);
	});

	it("keeps LoopPlaneStack mounted whenever the route has no data error", () => {
		expect(pageSource).toMatch(/\{#if !data\.error\}[\s\S]*?<LoopPlaneStack/);
		expect(pageSource).not.toMatch(/hasBucketPlanes/);
		expect(pageSource).not.toMatch(/LoopSurfacePlane/);
	});

	it("never imports knowledge_home from the loop route (§8 single-emission)", () => {
		expect(pageSource).not.toMatch(/\$lib\/connect\/knowledge_home/);
		expect(pageSource).not.toMatch(/trackHomeAction/);
	});

	it("routes external Open CTA into the SPA /articles/ reader instead of window.open", () => {
		// fb.md §5 / ADR-000875: Loop's Open intent goes through the in-app
		// reader so popup-blocker races and external-tab bounces are gone.
		expect(pageSource).toMatch(
			/goto\([\s\S]*?`\/articles\/\$\{encodeURIComponent\(entry\.entryKey\)\}/,
		);
		expect(pageSource).not.toMatch(/window\.open\(href, "_blank"/);
	});

	it("routes Review Open through the same SPA reader helper as foreground Open", () => {
		expect(pageSource).toMatch(
			/function onReviewOpen\(entry: KnowledgeLoopEntryData\)[\s\S]*?onEntryOpen\(entry\)/,
		);
		expect(pageSource).not.toMatch(
			/function onReviewOpen\(entry: KnowledgeLoopEntryData\)[\s\S]*?goto\(href\)/,
		);
	});

	it("Open CTA stays enabled and becomes 'Open · resolve url' when source URL is missing", () => {
		// Open recoverable (Knowledge Loop 体験回復 plan, Pillar 2A): NN/G's
		// rule that disabled buttons hide the reason and offer no recovery.
		// The button stays enabled; the secondary label signals that a BFF
		// lookup will fire on click. Pin both halves of the contract.
		expect(pageSource).not.toMatch(/disabled=\{!activeEntrySourceUrl\}/);
		expect(pageSource).toMatch(/Open · resolve url/);
		expect(pageSource).toMatch(/loop-open-resolve-error/);
	});

	it("delegates source-URL resolution to the shared resolveLoopSourceUrl helper", () => {
		// `actTargets[].route` is an internal SPA path the projector emits; it
		// MUST NOT be returned as a source URL or threaded through `?url=`.
		// The helper centralises the sourceUrl-first / evidenceRefs-fallback
		// logic and applies safeArticleHref defense.
		expect(pageSource).toMatch(/from\s+["']\$lib\/utils\/loop-source-url["']/);
		expect(pageSource).toMatch(/resolveLoopSourceUrl\s*\(/);
		// The pre-fix conflation: `if (article?.route) return article.route;`
		// returned the internal SPA path as a URL fallback. Must be gone.
		expect(pageSource).not.toMatch(
			/article\?\.route\s*\)\s*return\s+article\.route/,
		);
	});

	it("renders the Open command unconditionally in workspace-actions (Boyd IG&C)", () => {
		// Open is the universal Act affordance — reachable from every OODA stage,
		// never gated behind a `selectedStageName === "decide"/"act"` branch. The
		// old stage-gated Open was the dead-button bug: a fresh observe entry
		// could not be opened without first walking the loop by hand. It still
		// shows the "Open" / "Open · resolve url" label off `activeEntrySourceUrl`
		// and re-uses the inline resolve-error surface.
		expect(pageSource).toMatch(/class="workspace-actions"/);
		expect(pageSource).toMatch(/onWorkspaceOpen\(activeEntry\)/);
		expect(pageSource).toMatch(
			/activeEntrySourceUrl \? "Open" : "Open · resolve url"/,
		);
		expect(pageSource).toMatch(/loop-open-resolve-error/);
		// The stage gate that hid Open from observe / orient entries must be gone.
		expect(pageSource).not.toMatch(
			/selectedStageName === "decide" && activeEntry\.decisionOptions/,
		);
		// And no hand-walk "advance to next stage" stepper button remains in the
		// workspace command surface (the pipeline-myth UX the redesign removes).
		expect(pageSource).not.toMatch(/stageLabel\(nextStage\(activeEntry\)\)/);
	});

	it("emits a Revisited aria-live confirmation when same-stage intent_signal fires", () => {
		// ADR-924 backend fix landed: Revisit is accepted as a same-stage
		// intent_signal. But same-stage = `data-stage` does not change, so the
		// user keeps clicking — 13.7s / 10 clicks observed in production. Add
		// a polite aria-live status the assistive-tech surface sees as
		// "Revisited" and that we can drive a 1.5s visible flash off the same
		// state for sighted users.
		expect(pageSource).toMatch(/justActed/);
		expect(pageSource).toMatch(/aria-live=["']polite["']/);
		expect(pageSource).toMatch(/data-testid=["']loop-revisited-toast["']/);
		// Toast is rendered conditionally on the state flag (so it disappears
		// after the timeout). Confirm the conditional + reset both exist.
		expect(pageSource).toMatch(/\{#if justActed\}/);
		// The reset must be timed (not manual) so the toast clears itself.
		expect(pageSource).toMatch(
			/setTimeout\([\s\S]{0,200}justActed\s*=\s*false/,
		);
	});
});

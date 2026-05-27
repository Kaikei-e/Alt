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

	it("excludes the workspace spotlight entry from the NOW plane bench (spotlight + bench)", () => {
		// The .ooda-workspace section already renders activeEntry (foreground[0])
		// with its own action buttons. The NOW plane below MUST NOT render that
		// same entry again as a LoopEntryTile, or the same article shows two
		// stacked sets of REVISIT/ASK/SNOOZE controls (regression reported
		// 2026-05-27 via screenshot). The {#each} that feeds the foreground tiles
		// must filter on entryKey against activeEntry.
		expect(pageSource).toMatch(
			/foreground[\s\S]*?\.filter\([\s\S]*?activeEntry\?\.entryKey/,
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

	it("renders Open Article CTA in DECIDE stage workspace-actions", () => {
		// ADR-924 follow-up: Ask 後の entry は DECIDE で停滞するため、Open は
		// ACT stage 限定だと user が article を開く動線を見つけられない。
		// `workspace-actions` の DECIDE 分岐に `onWorkspaceOpen` を呼ぶボタンを
		// 追加し、`activeEntrySourceUrl` の有無に応じて "Open" / "Open · resolve url"
		// を出し分ける (Open recoverable パターン §Pillar 2A と整合)。
		const decideBranch = pageSource.match(
			/selectedStageName === "decide" && activeEntry\.decisionOptions\.length > 0[\s\S]{0,1500}/,
		);
		expect(decideBranch).not.toBeNull();
		const body = decideBranch?.[0] ?? "";
		expect(body).toMatch(/onWorkspaceOpen\(activeEntry\)/);
		expect(body).toMatch(
			/activeEntrySourceUrl \? "Open" : "Open · resolve url"/,
		);
		// inline error surface re-used from ACT branch so the BFF resolution
		// failure has the same recovery affordance the user already knows.
		expect(body).toMatch(/loop-open-resolve-error/);
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

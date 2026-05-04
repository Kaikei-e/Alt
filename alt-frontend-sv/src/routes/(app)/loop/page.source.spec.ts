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

	it("derives bucket planes from hook-owned bucketEntries", () => {
		expect(pageSource).toMatch(
			/const bucketEntries = \$derived\(loop\.bucketEntries\)/,
		);
		expect(pageSource).not.toMatch(
			/const bucketEntries = \$derived\(data\.loop\?\.bucketEntries/,
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
});

import { describe, expect, it } from "vitest";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

/**
 * Structural guard for /loop/+page.svelte.
 *
 * Pins the PR-L2 wiring: the page must instantiate `useKnowledgeLoop`, attach
 * the `observeTiles` action to the foreground container, and forward the hook
 * actions (onTransition / onDismiss / canTransition / isInFlight) into each
 * tile. Runtime rendering is exercised by the Playwright spec.
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

	it("attaches the observeTiles action to a container", () => {
		expect(pageSource).toMatch(/use:observeTiles/);
		expect(pageSource).toMatch(/onObserve/);
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

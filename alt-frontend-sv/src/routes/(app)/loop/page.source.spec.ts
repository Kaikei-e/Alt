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

	it("never imports knowledge_home from the loop route (§8 single-emission)", () => {
		expect(pageSource).not.toMatch(/\$lib\/connect\/knowledge_home/);
		expect(pageSource).not.toMatch(/trackHomeAction/);
	});
});

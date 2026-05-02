import { describe, expect, it } from "vitest";
import type { KnowledgeLoopEntryData } from "$lib/connect/knowledge_loop";
import { resolveLoopSourceUrl } from "./loop-source-url";

/**
 * Pure-function tests for resolveLoopSourceUrl. The function returns the
 * external HTTPS source URL of an entry (used by the SPA reader as `?url=`),
 * or null when no public-internet URL is available.
 *
 * Contract:
 *   - `actTargets[].sourceUrl` is the canonical input. `route` is the internal
 *     SPA path and MUST NOT be returned as a URL.
 *   - `whyPrimary.evidenceRefs[0].refId` is a fallback only when it is
 *     itself a valid public HTTPS URL.
 *   - Anything failing safeArticleHref (private host, javascript:, …) returns
 *     null and the UI must gracefully disable the Open command.
 */

const baseEntry: KnowledgeLoopEntryData = {
	entryKey: "loop-test-1",
	sourceItemKey: "article:test-1",
	proposedStage: "act",
	surfaceBucket: "now",
	projectionRevision: 1,
	projectionSeqHiwater: 1,
	freshnessAt: "2026-04-23T10:00:00Z",
	whyPrimary: {
		kind: "source_why",
		text: "test entry",
		evidenceRefs: [],
	},
	dismissState: "active",
	renderDepthHint: 2,
	loopPriority: "critical",
	decisionOptions: [],
	actTargets: [],
};

describe("resolveLoopSourceUrl", () => {
	it("returns actTargets[].sourceUrl when it is a public HTTPS URL", () => {
		const entry: KnowledgeLoopEntryData = {
			...baseEntry,
			actTargets: [
				{
					targetType: "article",
					targetRef: "art-1",
					route: "/articles/art-1",
					sourceUrl: "https://example.com/post",
				},
			],
		};
		expect(resolveLoopSourceUrl(entry)).toBe("https://example.com/post");
	});

	it("returns null when only route is set (route is internal SPA path)", () => {
		const entry: KnowledgeLoopEntryData = {
			...baseEntry,
			actTargets: [
				{
					targetType: "article",
					targetRef: "art-1",
					route: "/articles/art-1",
				},
			],
		};
		expect(resolveLoopSourceUrl(entry)).toBeNull();
	});

	it("rejects javascript: scheme in sourceUrl", () => {
		const entry: KnowledgeLoopEntryData = {
			...baseEntry,
			actTargets: [
				{
					targetType: "article",
					targetRef: "art-1",
					route: "/articles/art-1",
					sourceUrl: "javascript:alert(1)",
				},
			],
		};
		expect(resolveLoopSourceUrl(entry)).toBeNull();
	});

	it("rejects private-host sourceUrl (SSRF defense)", () => {
		const entry: KnowledgeLoopEntryData = {
			...baseEntry,
			actTargets: [
				{
					targetType: "article",
					targetRef: "art-1",
					route: "/articles/art-1",
					sourceUrl: "http://169.254.169.254/latest/meta-data",
				},
			],
		};
		expect(resolveLoopSourceUrl(entry)).toBeNull();
	});

	it("falls back to whyPrimary.evidenceRefs[0].refId when it is a public HTTPS URL", () => {
		const entry: KnowledgeLoopEntryData = {
			...baseEntry,
			whyPrimary: {
				...baseEntry.whyPrimary,
				evidenceRefs: [
					{ refId: "https://example.com/evidence", label: "primary" },
				],
			},
			actTargets: [
				{
					targetType: "article",
					targetRef: "art-1",
					route: "/articles/art-1",
				},
			],
		};
		expect(resolveLoopSourceUrl(entry)).toBe("https://example.com/evidence");
	});

	it("does not fall back to evidenceRefs[0] when refId is not a public URL", () => {
		const entry: KnowledgeLoopEntryData = {
			...baseEntry,
			whyPrimary: {
				...baseEntry.whyPrimary,
				evidenceRefs: [{ refId: "art-1", label: "article" }],
			},
		};
		expect(resolveLoopSourceUrl(entry)).toBeNull();
	});

	it("returns null when actTargets is empty and evidenceRefs[0] is missing", () => {
		expect(resolveLoopSourceUrl(baseEntry)).toBeNull();
	});

	it("ignores non-article targetType when reading sourceUrl", () => {
		const entry: KnowledgeLoopEntryData = {
			...baseEntry,
			actTargets: [
				{
					targetType: "recap",
					targetRef: "recap-1",
					route: "/recap/topic/recap-1",
					sourceUrl: "https://example.com/recap-not-article",
				},
			],
		};
		expect(resolveLoopSourceUrl(entry)).toBeNull();
	});
});

import type { KnowledgeLoopEntryData } from "$lib/connect/knowledge_loop";
import { safeArticleHref } from "./safeHref";

/**
 * resolveLoopSourceUrl: returns the external HTTPS source URL for a Knowledge
 * Loop entry, or null when no public-internet URL is available.
 *
 * Contract:
 *   - `actTargets[].sourceUrl` (where targetType is "article") is the canonical
 *     input. The projector emits this from the event payload (reproject-safe).
 *   - `actTargets[].route` is the *internal* SPA navigation target
 *     (e.g. `/articles/<id>`). It is display-only and MUST NOT be returned as
 *     a URL — doing so was the regression in ACT Open that caused the SPA
 *     reader to bail with "No article URL provided".
 *   - When sourceUrl is missing (legacy projection rows), fall back to
 *     `whyPrimary.evidenceRefs[0].refId` if it is itself a public HTTPS URL.
 *   - Both candidates pass through {@link safeArticleHref}, which enforces
 *     scheme allowlist + private-host SSRF rejection.
 *
 * The Loop page wires this into the Open command: when the result is null, the
 * UI must disable the Open button gracefully (`aria-label="Source URL
 * unavailable"`) instead of navigating into a broken reader state.
 */
export function resolveLoopSourceUrl(
	entry: KnowledgeLoopEntryData,
): string | null {
	const article = entry.actTargets.find((t) => t.targetType === "article");
	const fromActTarget = safeArticleHref(article?.sourceUrl);
	if (fromActTarget) return fromActTarget;

	const refId = entry.whyPrimary.evidenceRefs[0]?.refId;
	const fromEvidence = safeArticleHref(refId);
	if (fromEvidence) return fromEvidence;

	return null;
}

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

/**
 * Async resolver for the Knowledge Loop "Open · resolve url" recovery path.
 *
 * Tries the sync resolver first; if it succeeds the fetcher is never called.
 * Otherwise — and only when the entry has an article-typed actTarget — calls
 * the BFF lookup with the article's `target_ref`. On lookup failure or a URL
 * that fails `safeArticleHref` (defence-in-depth), returns null so the caller
 * can render an inline error.
 *
 * The fetcher is injected so the same function works in the page (real fetch)
 * and unit tests (vi.fn). It is expected to throw on non-2xx responses; the
 * caller already maps Connect codes to inline-error wording.
 */
export async function resolveLoopSourceUrlAsync(
	entry: KnowledgeLoopEntryData,
	fetcher: (articleId: string) => Promise<string>,
): Promise<string | null> {
	const sync = resolveLoopSourceUrl(entry);
	if (sync) return sync;

	const article = entry.actTargets.find((t) => t.targetType === "article");
	const articleId = article?.targetRef;
	if (!articleId) return null;

	try {
		const fetched = await fetcher(articleId);
		return safeArticleHref(fetched);
	} catch {
		return null;
	}
}

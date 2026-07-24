import { sanitizeHrefUrl } from "$lib/utils/urlSafety";

export type CitationKindName = "UNSPECIFIED" | "WEB" | "ARTICLE" | "SUMMARY";

export type CitationLinkInput = {
	kind: CitationKindName;
	url: string;
	refId: string;
};

/**
 * Resolve the click target for a citation rendered in the Augur citation rail.
 *
 * The rules are kind-driven, never url-shape-driven, because the browser
 * resolves a bare UUID `<a href="abc-...">` against the current `/augur/<id>`
 * route and silently produces a dead link. Legacy payloads without a kind
 * must therefore render without a link rather than gambling on the contents
 * of `url`.
 */
export function citationHref(c: CitationLinkInput): string | undefined {
	switch (c.kind) {
		case "WEB":
			return sanitizeHrefUrl(c.url);
		case "ARTICLE":
		case "SUMMARY":
			return c.refId ? `/articles/${c.refId}` : undefined;
		default:
			return undefined;
	}
}

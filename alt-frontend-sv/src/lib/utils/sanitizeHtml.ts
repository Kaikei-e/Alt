import DOMPurify from "isomorphic-dompurify";

// Add hook to force target="_blank" and rel="noopener noreferrer" on all links
// This ensures external links open in new tabs safely
DOMPurify.addHook("afterSanitizeAttributes", (node) => {
	if (node.tagName === "A") {
		node.setAttribute("target", "_blank");
		node.setAttribute("rel", "noopener noreferrer");
	}
});

/**
 * Sanitizes HTML content using DOMPurify to prevent XSS attacks.
 * Preserves structural elements like headers, lists, code blocks, images, and links
 * while removing potentially dangerous content like scripts and event handlers.
 *
 * All links (<a> tags) are automatically modified to open in new tabs with
 * target="_blank" and rel="noopener noreferrer" for security.
 *
 * @param html - Raw HTML string to sanitize
 * @returns Sanitized HTML string safe for rendering with {@html}
 */
export function sanitizeHtml(html: string): string {
	if (!html) return "";

	return DOMPurify.sanitize(html, {
		// Allowed tags - preserve rich content structure
		// NOTE: img tags are intentionally excluded as Alt doesn't fetch images
		// and img tags are a common XSS vector (onerror, onload events)
		ALLOWED_TAGS: [
			// Structural elements
			"p",
			"br",
			"div",
			"span",
			"article",
			"section",
			// Headers
			"h1",
			"h2",
			"h3",
			"h4",
			"h5",
			"h6",
			// Lists
			"ul",
			"ol",
			"li",
			// Quotes and code
			"blockquote",
			"pre",
			"code",
			// Text formatting
			"strong",
			"em",
			"b",
			"i",
			"u",
			"s",
			"del",
			"ins",
			"mark",
			"sub",
			"sup",
			// Links (no images - security risk and not fetched anyway)
			"a",
			// Tables
			"table",
			"thead",
			"tbody",
			"tfoot",
			"tr",
			"th",
			"td",
			"caption",
			// Other
			"hr",
			"figure",
			"figcaption",
			"dl",
			"dt",
			"dd",
		],
		// Allowed attributes - only essential ones (no src/alt since img is removed)
		// target and rel are needed for opening links in new tabs safely
		ALLOWED_ATTR: ["href", "title", "class", "target", "rel"],
		// Allow the hook to add target and rel attributes
		ADD_ATTR: ["target", "rel"],
		// Prevent data: URLs which can be used for XSS
		ALLOW_DATA_ATTR: false,
		// Allow only safe URL protocols (https/http only, mailto excluded for RSS context)
		// Note: hyphen escaped as \- to avoid ambiguous character range (CodeQL js/overly-large-range)
		ALLOWED_URI_REGEXP:
			/^(?:https?:|[^a-z]|[a-z+.\-]+(?:[^a-z+.\-:]|$))/i,
	});
}

/**
 * Validate a URL before it is bound to an `href` attribute.
 *
 * Only `http:`/`https:` URLs are allowed. Any other scheme (`javascript:`,
 * `vbscript:`, `data:`, `file:`, `ftp:`, ...) is rejected, since these values
 * can originate from external, attacker-controlled sources (RSS article
 * links, RAG/Recap citations) and would otherwise let a malicious citation
 * URL execute script on click.
 */
export function sanitizeHrefUrl(url: string | null | undefined): string | undefined {
	if (!url) return undefined;
	const urlPattern = /^https?:\/\//i;
	if (!urlPattern.test(url)) return undefined;
	const dangerousProtocols = /^(javascript|vbscript|data|ftp|file):/i;
	if (dangerousProtocols.test(url)) return undefined;
	return url;
}

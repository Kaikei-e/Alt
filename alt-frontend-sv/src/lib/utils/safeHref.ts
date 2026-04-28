/**
 * safeArticleHref: returns a sanitized HTTP(S) URL or null when the input
 * is empty, malformed, or carries a non-allowlisted scheme.
 *
 * Allowlist: `http`, `https`. Everything else (`javascript`, `data`,
 * `file`, `vbscript`, protocol-relative `//host`, relative paths) is
 * rejected. Casing and leading whitespace are normalised before
 * inspection.
 *
 * Used as the canonical defense-in-depth layer for any external URL the
 * Knowledge Home / Loop UIs may render as `<a href>`. The corresponding
 * Go-side guard lives in alt-backend's projector (`isHTTPURL`) and the
 * sovereign-side patch SQL has an empty-string reject — three layers,
 * all aligned, per docs/glossary/ubiquitous-language.md and the
 * security-auditor F-001 finding for ADR-000867.
 */
export function safeArticleHref(
	link: string | null | undefined,
): string | null {
	if (link == null) return null;
	const trimmed = link.trim();
	if (trimmed === "") return null;

	let parsed: URL;
	try {
		parsed = new URL(trimmed);
	} catch {
		return null;
	}

	const scheme = parsed.protocol.toLowerCase();
	if (scheme !== "http:" && scheme !== "https:") return null;
	if (!parsed.host) return null;

	return parsed.toString();
}

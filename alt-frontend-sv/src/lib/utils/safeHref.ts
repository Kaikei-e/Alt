/**
 * safeArticleHref: returns a sanitized HTTP(S) URL or null when the input
 * is empty, malformed, or carries a non-allowlisted scheme.
 *
 * Allowlist: `http`, `https`. Everything else (`javascript`, `data`,
 * `file`, `vbscript`, protocol-relative `//host`, relative paths) is
 * rejected. Casing and leading whitespace are normalised before
 * inspection. Private, loopback, and link-local hosts are rejected
 * via {@link isPublicHost} as a baseline SSRF defense — but the BFF
 * MUST still apply its own server-side allowlist before any outbound
 * fetch (this guard runs in the browser).
 *
 * Used as the canonical defense-in-depth layer for any external URL the
 * Knowledge Home / Loop UIs may render as `<a href>` or thread through
 * `?url=` to the SPA reader. The corresponding Go-side guard lives in
 * alt-backend's projector (`isHTTPURL`) and the sovereign-side patch SQL
 * has an empty-string reject — three layers, all aligned, per
 * docs/glossary/ubiquitous-language.md and the security-auditor F-001
 * finding for ADR-000867.
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
	if (!isPublicHost(parsed)) return null;

	return parsed.toString();
}

/**
 * isPublicHost: returns true when the URL's host is reachable as a
 * public-internet endpoint, false for loopback / private / link-local /
 * unspecified addresses. Used by safeArticleHref as a baseline SSRF
 * defense for `?url=` flows.
 *
 * NOT a complete SSRF guard. The BFF's outbound fetch must still apply
 * its own allowlist; DNS rebinding can flip a public hostname to a
 * private IP after this check. See ADR security audit Medium #3.
 *
 * Recognised non-public ranges:
 *   - localhost / *.localdomain
 *   - 127.0.0.0/8 (loopback)
 *   - 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16 (RFC 1918 private)
 *   - 169.254.0.0/16 (link-local — AWS / GCP metadata)
 *   - 0.0.0.0/8 (unspecified)
 *   - ::1 (IPv6 loopback)
 *   - fe80::/10 (IPv6 link-local)
 *   - fc00::/7 (IPv6 unique-local)
 */
export function isPublicHost(parsed: URL): boolean {
	const host = parsed.hostname;
	if (!host) return false;

	const lowered = host.toLowerCase();
	if (lowered === "localhost" || lowered.endsWith(".localdomain")) {
		return false;
	}

	// IPv6 literal — different runtimes return `[::1]` (Node/Bun) or
	// `::1` (some browsers). Strip brackets before classifying.
	if (lowered.includes(":")) {
		const v6 =
			lowered.startsWith("[") && lowered.endsWith("]")
				? lowered.slice(1, -1)
				: lowered;
		return isPublicIPv6(v6);
	}

	// IPv4 literal (4 dotted decimal octets).
	const ipv4 = parseIPv4(lowered);
	if (ipv4 !== null) {
		return isPublicIPv4(ipv4);
	}

	// Hostname (DNS name) — accept anything not matching the literal cases.
	// DNS rebinding to a private IP is the BFF's problem (server-side check).
	return true;
}

function parseIPv4(host: string): [number, number, number, number] | null {
	const parts = host.split(".");
	if (parts.length !== 4) return null;
	const out: number[] = [];
	for (const part of parts) {
		if (!/^\d+$/.test(part)) return null;
		const n = Number(part);
		if (n < 0 || n > 255) return null;
		out.push(n);
	}
	return [out[0], out[1], out[2], out[3]] as [number, number, number, number];
}

function isPublicIPv4(octets: [number, number, number, number]): boolean {
	const [a, b] = octets;
	if (a === 10) return false; // 10.0.0.0/8
	if (a === 127) return false; // 127.0.0.0/8 (loopback)
	if (a === 0) return false; // 0.0.0.0/8 (unspecified)
	if (a === 169 && b === 254) return false; // 169.254.0.0/16 (link-local)
	if (a === 172 && b >= 16 && b <= 31) return false; // 172.16.0.0/12
	if (a === 192 && b === 168) return false; // 192.168.0.0/16
	return true;
}

function isPublicIPv6(host: string): boolean {
	// Loopback ::1.
	if (host === "::1") return false;
	// Link-local fe80::/10 — first hextet starts fe8/fe9/fea/feb.
	if (/^fe[89ab]/i.test(host)) return false;
	// Unique-local fc00::/7 — first hextet starts fc or fd.
	if (/^f[cd]/i.test(host)) return false;
	// Unspecified ::.
	if (host === "::") return false;
	return true;
}

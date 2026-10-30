/**
 * Server-only SSRF validation utility
 * Prevents Server-Side Request Forgery attacks by validating URLs
 * Based on backend implementation (ssrf_validator.go)
 */

export class SSRFValidationError extends Error {
	constructor(
		message: string,
		public readonly type: string,
		public readonly details?: Record<string, unknown>,
	) {
		super(message);
		this.name = "SSRFValidationError";
	}
}

const METADATA_ENDPOINTS = [
	"169.254.169.254", // AWS/Azure/GCP metadata
	"metadata.google.internal", // GCP metadata
	"100.100.100.200", // Alibaba Cloud
	"192.0.0.192", // Oracle Cloud
];

const INTERNAL_DOMAIN_SUFFIXES = [
	".local",
	".localdomain",
	".internal",
	".corp",
	".lan",
	".intranet",
	".test",
	".localhost",
	".cluster.local",
];

/**
 * Strips trailing DNS root dots, lowercases, and unwraps IPv6 brackets.
 * If the address is an IPv4-mapped IPv6 literal (e.g. `::ffff:127.0.0.1` or
 * WHATWG-normalized `::ffff:7f00:1`), extracts the embedded IPv4 address so
 * all IPv4 checks apply uniformly.
 */
function normalizeHost(rawHostname: string): string {
	let host = rawHostname.trim().toLowerCase().replace(/\.+$/, "");
	if (host.startsWith("[") && host.endsWith("]")) {
		host = host.slice(1, -1);
	}
	const mappedIpv4 = extractMappedIPv4(host);
	if (mappedIpv4) {
		return mappedIpv4;
	}
	return host;
}

function extractMappedIPv4(host: string): string | null {
	const match = /^(?:::ffff:|(?:0+:){5}ffff:)(.+)$/i.exec(host);
	if (!match?.[1]) return null;
	const rest = match[1];
	if (/^\d+\.\d+\.\d+\.\d+$/.test(rest)) {
		return rest;
	}
	const hexMatch = /^([0-9a-f]{1,4}):([0-9a-f]{1,4})$/i.exec(rest);
	if (hexMatch?.[1] && hexMatch[2]) {
		const hi = Number.parseInt(hexMatch[1], 16);
		const lo = Number.parseInt(hexMatch[2], 16);
		return `${(hi >> 8) & 0xff}.${hi & 0xff}.${(lo >> 8) & 0xff}.${lo & 0xff}`;
	}
	return null;
}

/**
 * Validates URL for SSRF attacks
 * Blocks private IPs, metadata endpoints, and internal domains
 */
export function validateUrlForSSRF(url: string): void {
	if (!url || typeof url !== "string") {
		throw new SSRFValidationError("URL is required", "INVALID_URL");
	}

	let parsedUrl: URL;
	try {
		parsedUrl = new URL(url);
	} catch (error) {
		throw new SSRFValidationError("Invalid URL format", "INVALID_URL_FORMAT", {
			error: String(error),
		});
	}

	// Validate scheme (only HTTP/HTTPS allowed)
	if (parsedUrl.protocol !== "http:" && parsedUrl.protocol !== "https:") {
		throw new SSRFValidationError(
			"Only HTTP and HTTPS schemes are allowed",
			"INVALID_SCHEME",
			{ scheme: parsedUrl.protocol },
		);
	}

	// Normalize hostname (strips trailing dots, brackets, and unwraps IPv4-mapped IPv6)
	const hostname = normalizeHost(parsedUrl.hostname);
	if (!hostname) {
		throw new SSRFValidationError("Invalid URL format", "INVALID_URL_FORMAT", {
			hostname: parsedUrl.hostname,
		});
	}

	// Check metadata endpoints (highest priority)
	for (const endpoint of METADATA_ENDPOINTS) {
		if (hostname === endpoint || hostname.startsWith(`${endpoint}:`)) {
			throw new SSRFValidationError(
				"Access to metadata endpoint not allowed",
				"METADATA_ENDPOINT_BLOCKED",
				{ hostname, endpoint },
			);
		}
	}

	// Check internal domains and single-label Docker service hostnames (excluding localhost which is classified as LOOPBACK_BLOCKED below)
	if (
		hostname !== "localhost" &&
		!hostname.includes(".") &&
		!hostname.includes(":")
	) {
		throw new SSRFValidationError(
			"Internal single-label hostname not allowed",
			"INTERNAL_DOMAIN_BLOCKED",
			{ hostname },
		);
	}
	for (const suffix of INTERNAL_DOMAIN_SUFFIXES) {
		if (hostname.endsWith(suffix)) {
			throw new SSRFValidationError(
				"Internal domain not allowed",
				"INTERNAL_DOMAIN_BLOCKED",
				{ hostname, suffix },
			);
		}
	}

	// Validate port (only allow common web ports)
	const allowedPorts = new Set(["80", "443", "8080", "8443"]);
	if (parsedUrl.port && !allowedPorts.has(parsedUrl.port)) {
		throw new SSRFValidationError("Port not allowed", "INVALID_PORT", {
			port: parsedUrl.port,
		});
	}

	// Validate IP addresses
	if (isPrivateIP(hostname)) {
		throw new SSRFValidationError(
			"Private IP address not allowed",
			"PRIVATE_IP_BLOCKED",
			{ hostname },
		);
	}

	// Validate loopback addresses (including 0.0.0.0 / :: unspecified)
	if (isLoopback(hostname)) {
		throw new SSRFValidationError(
			"Loopback address not allowed",
			"LOOPBACK_BLOCKED",
			{ hostname },
		);
	}

	// Validate link-local addresses
	if (isLinkLocal(hostname)) {
		throw new SSRFValidationError(
			"Link-local address not allowed",
			"LINK_LOCAL_BLOCKED",
			{ hostname },
		);
	}
}

export const validateUrlForAnalysis = validateUrlForSSRF;

/**
 * Returns true if the given hostname is private, loopback, link-local,
 * unspecified, metadata, or internal.
 */
export function isPrivateHost(rawHostname: string): boolean {
	const hostname = normalizeHost(rawHostname);
	if (!hostname) return true;
	for (const endpoint of METADATA_ENDPOINTS) {
		if (hostname === endpoint || hostname.startsWith(`${endpoint}:`)) {
			return true;
		}
	}
	if (!hostname.includes(".") && !hostname.includes(":")) {
		return true;
	}
	for (const suffix of INTERNAL_DOMAIN_SUFFIXES) {
		if (hostname.endsWith(suffix)) {
			return true;
		}
	}
	return isPrivateIP(hostname) || isLoopback(hostname) || isLinkLocal(hostname);
}

/**
 * Checks if hostname is a private IP address (RFC1918 / RFC4193 / CGNAT)
 */
function isPrivateIP(hostname: string): boolean {
	const host = normalizeHost(hostname);

	// IPv4 private and reserved ranges
	const privateIPv4Patterns = [
		/^10\./, // 10.0.0.0/8
		/^172\.(1[6-9]|2[0-9]|3[0-1])\./, // 172.16.0.0/12
		/^192\.168\./, // 192.168.0.0/16
		/^100\.(6[4-9]|[7-9]\d|1[01]\d|12[0-7])\./, // 100.64.0.0/10 (CGNAT)
		/^192\.0\.0\./, // 192.0.0.0/24 (IETF Protocol Assignments)
	];

	for (const pattern of privateIPv4Patterns) {
		if (pattern.test(host)) {
			return true;
		}
	}

	// IPv6 unique-local range fc00::/7 (fc00:: through fdff::)
	if (/^f[cd][0-9a-f]{0,2}:/i.test(host)) {
		return true;
	}

	return false;
}

/**
 * Checks if hostname is a loopback or unspecified address
 */
function isLoopback(hostname: string): boolean {
	const host = normalizeHost(hostname);
	const loopbackPatterns = [
		/^127\./, // 127.0.0.0/8
		/^0\./, // 0.0.0.0/8 unspecified (routes to loopback on Linux)
		/^::1$/, // IPv6 loopback
		/^::$/, // IPv6 unspecified
		/^(?:0+:){7}0*[01]$/i, // Full-form IPv6 :: or ::1
		/^localhost$/,
	];

	for (const pattern of loopbackPatterns) {
		if (pattern.test(host)) {
			return true;
		}
	}

	return false;
}

/**
 * Checks if hostname is a link-local address
 */
function isLinkLocal(hostname: string): boolean {
	const host = normalizeHost(hostname);
	// IPv4 link-local: 169.254.0.0/16
	if (/^169\.254\./.test(host)) {
		return true;
	}

	// IPv6 link-local: fe80::/10
	if (/^fe[89ab][0-9a-f]{0,1}:/i.test(host)) {
		return true;
	}

	return false;
}

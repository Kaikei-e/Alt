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

	// Validate hostname
	const hostname = parsedUrl.hostname.toLowerCase();

	// Check metadata endpoints (highest priority)
	const metadataEndpoints = [
		"169.254.169.254", // AWS/Azure/GCP metadata
		"metadata.google.internal", // GCP metadata
		"100.100.100.200", // Alibaba Cloud
		"192.0.0.192", // Oracle Cloud
	];

	for (const endpoint of metadataEndpoints) {
		if (hostname === endpoint || hostname.startsWith(`${endpoint}:`)) {
			throw new SSRFValidationError(
				"Access to metadata endpoint not allowed",
				"METADATA_ENDPOINT_BLOCKED",
				{ hostname, endpoint },
			);
		}
	}

	// Check internal domains
	const internalDomainSuffixes = [
		".local",
		".internal",
		".corp",
		".lan",
		".intranet",
		".test",
		".localhost",
		".cluster.local",
	];

	for (const suffix of internalDomainSuffixes) {
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

	// Validate loopback addresses
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

/**
 * Checks if hostname is a private IP address (RFC1918)
 */
function isPrivateIP(hostname: string): boolean {
	// IPv4 private ranges
	const privateIPv4Patterns = [
		/^10\./, // 10.0.0.0/8
		/^172\.(1[6-9]|2[0-9]|3[0-1])\./, // 172.16.0.0/12
		/^192\.168\./, // 192.168.0.0/16
	];

	for (const pattern of privateIPv4Patterns) {
		if (pattern.test(hostname)) {
			return true;
		}
	}

	// IPv6 private ranges (simplified check)
	if (hostname.startsWith("fc00:") || hostname.startsWith("fd00:")) {
		return true;
	}

	return false;
}

/**
 * Checks if hostname is a loopback address
 */
function isLoopback(hostname: string): boolean {
	const loopbackPatterns = [
		/^127\./, // 127.0.0.0/8
		/^::1$/, // IPv6 loopback
		/^localhost$/,
	];

	for (const pattern of loopbackPatterns) {
		if (pattern.test(hostname)) {
			return true;
		}
	}

	return false;
}

/**
 * Checks if hostname is a link-local address
 */
function isLinkLocal(hostname: string): boolean {
	// IPv4 link-local: 169.254.0.0/16
	if (/^169\.254\./.test(hostname)) {
		return true;
	}

	// IPv6 link-local: fe80::/10
	if (/^fe80:/i.test(hostname)) {
		return true;
	}

	return false;
}

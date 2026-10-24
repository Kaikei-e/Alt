import { describe, expect, it } from "vitest";
import {
	isPrivateHost,
	SSRFValidationError,
	validateUrlForAnalysis,
	validateUrlForSSRF,
} from "./ssrf-validator";

describe("ssrf-validator", () => {
	it.each([
		["https://example.com/article"],
		["http://example.org:8080/feed.xml"],
		["https://93.184.216.34/news"],
		["https://[2606:2800:220:1:248:1893:25c8:1946]/"],
	])("allows safe public URL %s", (url) => {
		expect(() => validateUrlForSSRF(url)).not.toThrow();
		expect(() => validateUrlForAnalysis(url)).not.toThrow();
		expect(isPrivateHost(new URL(url).hostname)).toBe(false);
	});

	it.each([
		["http://[::1]/", "LOOPBACK_BLOCKED"],
		["http://[::1]:8080/admin", "LOOPBACK_BLOCKED"],
		["http://[0:0:0:0:0:0:0:1]/", "LOOPBACK_BLOCKED"],
		["http://[::]/", "LOOPBACK_BLOCKED"],
		["http://[::ffff:127.0.0.1]/", "LOOPBACK_BLOCKED"],
		["http://[::ffff:7f00:1]/", "LOOPBACK_BLOCKED"],
		["http://[0:0:0:0:0:ffff:127.0.0.1]/", "LOOPBACK_BLOCKED"],
		["http://[::ffff:10.0.0.1]/", "PRIVATE_IP_BLOCKED"],
		["http://[::ffff:192.168.1.1]/", "PRIVATE_IP_BLOCKED"],
		["http://[::ffff:172.16.0.1]/", "PRIVATE_IP_BLOCKED"],
		[
			"http://[::ffff:169.254.169.254]/latest/meta-data/",
			"METADATA_ENDPOINT_BLOCKED",
		],
		["http://0.0.0.0/", "LOOPBACK_BLOCKED"],
		["http://0.1.2.3/", "LOOPBACK_BLOCKED"],
		["http://127.0.0.1/", "LOOPBACK_BLOCKED"],
		["http://localhost/", "LOOPBACK_BLOCKED"],
		["http://localhost./", "LOOPBACK_BLOCKED"],
		["http://localhost.../admin", "LOOPBACK_BLOCKED"],
		["http://[fc00::1]/", "PRIVATE_IP_BLOCKED"],
		["http://[fd12:3456:789a::1]/", "PRIVATE_IP_BLOCKED"],
		["http://[fe80::1]/", "LINK_LOCAL_BLOCKED"],
		["http://169.254.169.254/latest/meta-data/", "METADATA_ENDPOINT_BLOCKED"],
		[
			"http://metadata.google.internal./computeMetadata/v1/",
			"METADATA_ENDPOINT_BLOCKED",
		],
		["http://service.cluster.local./api", "INTERNAL_DOMAIN_BLOCKED"],
		["http://internal.corp/", "INTERNAL_DOMAIN_BLOCKED"],
		["file:///etc/passwd", "INVALID_SCHEME"],
		["http://example.com:6379/", "INVALID_PORT"],
	])("rejects %s with %s", (url, expectedType) => {
		let caught: unknown;
		try {
			validateUrlForSSRF(url);
		} catch (err) {
			caught = err;
		}
		expect(caught).toBeInstanceOf(SSRFValidationError);
		expect((caught as SSRFValidationError).type).toBe(expectedType);
	});

	it("isPrivateHost identifies bracketed IPv6, IPv4-mapped IPv6, 0.0.0.0, and trailing-dot localhost.", () => {
		expect(isPrivateHost("[::1]")).toBe(true);
		expect(isPrivateHost("[::ffff:127.0.0.1]")).toBe(true);
		expect(isPrivateHost("[::ffff:7f00:1]")).toBe(true);
		expect(isPrivateHost("0.0.0.0")).toBe(true);
		expect(isPrivateHost("localhost.")).toBe(true);
		expect(isPrivateHost("[fe80::1]")).toBe(true);
		expect(isPrivateHost("[fd00::1]")).toBe(true);
		expect(isPrivateHost("example.com")).toBe(false);
	});
});

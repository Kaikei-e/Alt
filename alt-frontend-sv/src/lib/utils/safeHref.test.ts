import { describe, expect, it } from "vitest";
import { isPublicHost, safeArticleHref } from "./safeHref";

describe("safeArticleHref", () => {
	it.each([
		["https://example.com/article", "https://example.com/article"],
		["http://example.com/article", "http://example.com/article"],
		["https://example.com:8080/path?q=1", "https://example.com:8080/path?q=1"],
		[
			"  https://example.com/leading-whitespace  ",
			"https://example.com/leading-whitespace",
		],
	])("accepts allowlisted scheme %p → %p", (input, expected) => {
		expect(safeArticleHref(input)).toBe(expected);
	});

	it.each([
		// Dangerous schemes — must NEVER render as href.
		["javascript:alert(1)"],
		["JaVaScRiPt:alert(1)"],
		["   javascript:alert(1)"],
		["data:text/html,<script>alert(1)</script>"],
		["file:///etc/passwd"],
		["vbscript:msgbox"],
		// Protocol-relative — historically used for open-redirect
		// (ADR closure af79c171f). URL constructor throws on these
		// without a base, which we treat as rejection.
		["//evil.com/redirect"],
		// Relative path — KnowledgeCard.url is for external link
		// only; relative routing belongs to a different component.
		["/articles/123"],
		// Schemes with no host — empty hostname is unsafe.
		["https://"],
		// Malformed.
		["htps:::"],
		["not a url"],
		// Empty / nullish.
		[""],
		["   "],
	])("rejects unsafe input %p", (input) => {
		expect(safeArticleHref(input)).toBeNull();
	});

	it("rejects null and undefined", () => {
		expect(safeArticleHref(null)).toBeNull();
		expect(safeArticleHref(undefined)).toBeNull();
	});

	// SSRF defense — `?url=` flows through this guard before the BFF receives
	// it, so a private/loopback/link-local host must never round-trip.
	it.each([
		["https://localhost/admin"],
		["https://localhost:3000/foo"],
		["http://127.0.0.1/secret"],
		["http://127.10.20.30/x"],
		["http://10.1.2.3/internal"],
		["http://10.255.255.254/x"],
		["http://172.16.0.1/x"],
		["http://172.31.255.255/x"],
		["http://192.168.0.1/x"],
		["http://192.168.255.255/x"],
		["http://169.254.169.254/latest/meta-data"],
		["http://[::1]/loopback"],
		["http://[fe80::1]/link-local"],
	])("rejects private/loopback/link-local host %p", (input) => {
		expect(safeArticleHref(input)).toBeNull();
	});
});

describe("isPublicHost", () => {
	it.each([
		["https://example.com"],
		["https://example.co.jp:443"],
		["https://93.184.216.34"],
		["https://[2001:db8::1]"],
	])("accepts public host %p", (input) => {
		const parsed = new URL(input);
		expect(isPublicHost(parsed)).toBe(true);
	});

	it.each([
		["https://localhost"],
		["http://localhost.localdomain"],
		["http://127.0.0.1"],
		["http://127.255.255.255"],
		["http://10.0.0.1"],
		["http://172.16.0.1"],
		["http://172.31.255.254"],
		["http://192.168.1.1"],
		["http://169.254.169.254"],
		["http://0.0.0.0"],
		["http://[::1]"],
		["http://[fe80::abcd]"],
		["http://[fc00::1]"],
	])("rejects private/loopback/link-local %p", (input) => {
		const parsed = new URL(input);
		expect(isPublicHost(parsed)).toBe(false);
	});

	it("rejects empty host", () => {
		// URL constructor with an HTTP scheme always supplies a host, so
		// this guards a future caller passing a non-HTTP parsed URL whose
		// host is empty (e.g. mailto:).
		const fake = { hostname: "" } as URL;
		expect(isPublicHost(fake)).toBe(false);
	});
});

import * as v from "valibot";
import { describe, expect, it, vi } from "vitest";
import { safeUrlSchema, validateUrl } from "./urlValidation";

describe("validateUrl", () => {
	describe("valid URLs", () => {
		it("accepts https URL", () => {
			expect(validateUrl("https://example.com")).toBe(true);
		});

		it("accepts http URL", () => {
			expect(validateUrl("http://example.com")).toBe(true);
		});

		it("accepts URL with path", () => {
			expect(validateUrl("https://example.com/path/to/page")).toBe(true);
		});

		it("accepts URL with query params", () => {
			expect(validateUrl("https://example.com?foo=bar&baz=qux")).toBe(true);
		});

		it("accepts URL with port", () => {
			expect(validateUrl("https://example.com:8080/api")).toBe(true);
		});

		it("accepts subdomain URLs", () => {
			expect(validateUrl("https://sub.domain.example.com")).toBe(true);
		});
	});

	describe("invalid protocols", () => {
		it("rejects javascript: URL", () => {
			expect(validateUrl("javascript:alert(1)")).toBe(false);
		});

		it("rejects data: URL", () => {
			expect(validateUrl("data:text/html,<script>alert(1)</script>")).toBe(
				false,
			);
		});

		it("rejects file: URL", () => {
			expect(validateUrl("file:///etc/passwd")).toBe(false);
		});

		it("rejects ftp: URL", () => {
			expect(validateUrl("ftp://example.com/file")).toBe(false);
		});

		it("rejects vbscript: URL", () => {
			expect(validateUrl("vbscript:msgbox(1)")).toBe(false);
		});

		it("rejects chrome: URL", () => {
			expect(validateUrl("chrome://settings")).toBe(false);
		});

		it("rejects about: URL", () => {
			expect(validateUrl("about:blank")).toBe(false);
		});
	});

	describe("hostname validation", () => {
		it("rejects URL without hostname", () => {
			expect(validateUrl("https://")).toBe(false);
		});

		it("rejects invalid domain format with special chars", () => {
			expect(validateUrl("https://exam!ple.com")).toBe(false);
		});

		it("accepts valid hyphenated domains", () => {
			expect(validateUrl("https://my-example-site.com")).toBe(true);
		});

		it("rejects domains starting with hyphen", () => {
			expect(validateUrl("https://-example.com")).toBe(false);
		});

		it("rejects domains ending with hyphen", () => {
			expect(validateUrl("https://example-.com")).toBe(false);
		});
	});

	describe("edge cases", () => {
		it("returns false for empty string", () => {
			expect(validateUrl("")).toBe(false);
		});

		it("returns false for null", () => {
			// @ts-expect-error Testing null input
			expect(validateUrl(null)).toBe(false);
		});

		it("returns false for undefined", () => {
			// @ts-expect-error Testing undefined input
			expect(validateUrl(undefined)).toBe(false);
		});

		it("trims whitespace before validation", () => {
			expect(validateUrl("  https://example.com  ")).toBe(true);
		});

		it("returns false for non-string input", () => {
			// @ts-expect-error Testing number input
			expect(validateUrl(12345)).toBe(false);
		});

		it("rejects URL-like strings without protocol", () => {
			expect(validateUrl("example.com")).toBe(false);
		});
	});

	describe("dangerous protocol detection in URL string", () => {
		it("rejects http URL containing javascript: in path", () => {
			expect(validateUrl("https://example.com/javascript:alert(1)")).toBe(
				false,
			);
		});

		it("rejects URL containing data: substring", () => {
			expect(validateUrl("https://example.com?redirect=data:text/html")).toBe(
				false,
			);
		});
	});
});

describe("safeUrlSchema", () => {
	it("parses valid URL successfully", () => {
		const result = v.safeParse(safeUrlSchema, "https://example.com");
		expect(result.success).toBe(true);
	});

	it("fails for invalid URL", () => {
		const result = v.safeParse(safeUrlSchema, "javascript:alert(1)");
		expect(result.success).toBe(false);
	});

	it("returns appropriate error message", () => {
		const result = v.safeParse(safeUrlSchema, "not-a-url");
		expect(result.success).toBe(false);
		if (!result.success) {
			expect(result.issues[0].message).toBe("Invalid or unsafe URL");
		}
	});
});

import { describe, expect, it } from "vitest";
import { sanitizeHrefUrl } from "./urlSafety";

describe("sanitizeHrefUrl", () => {
	it("allows https URLs", () => {
		expect(sanitizeHrefUrl("https://example.com/article")).toBe(
			"https://example.com/article",
		);
	});

	it("allows http URLs", () => {
		expect(sanitizeHrefUrl("http://example.com/article")).toBe(
			"http://example.com/article",
		);
	});

	it("rejects javascript: URLs", () => {
		expect(sanitizeHrefUrl("javascript:alert(1)")).toBeUndefined();
	});

	it("rejects vbscript: URLs", () => {
		expect(sanitizeHrefUrl("vbscript:msgbox(1)")).toBeUndefined();
	});

	it("rejects data: URLs", () => {
		expect(
			sanitizeHrefUrl("data:text/html,<script>alert(1)</script>"),
		).toBeUndefined();
	});

	it("rejects file: URLs", () => {
		expect(sanitizeHrefUrl("file:///etc/passwd")).toBeUndefined();
	});

	it("rejects protocol-relative and empty values", () => {
		expect(sanitizeHrefUrl("//evil.example.com")).toBeUndefined();
		expect(sanitizeHrefUrl("")).toBeUndefined();
		expect(sanitizeHrefUrl(undefined)).toBeUndefined();
		expect(sanitizeHrefUrl(null)).toBeUndefined();
	});

	it("is case-insensitive for dangerous schemes", () => {
		expect(sanitizeHrefUrl("JaVaScRiPt:alert(1)")).toBeUndefined();
	});
});

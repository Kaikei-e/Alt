import { describe, expect, it } from "vitest";
import { safeArticleHref } from "./safeHref";

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
});

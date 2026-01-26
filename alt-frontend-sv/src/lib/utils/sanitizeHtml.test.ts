import { describe, it, expect } from "vitest";
import { sanitizeHtml } from "./sanitizeHtml";

describe("sanitizeHtml", () => {
	it("returns empty string for empty input", () => {
		expect(sanitizeHtml("")).toBe("");
	});

	it("returns empty string for null/undefined", () => {
		expect(sanitizeHtml(null as unknown as string)).toBe("");
		expect(sanitizeHtml(undefined as unknown as string)).toBe("");
	});

	it("preserves paragraph tags", () => {
		const html = "<p>Hello world</p>";
		const result = sanitizeHtml(html);
		expect(result).toContain("<p>");
		expect(result).toContain("Hello world");
	});

	it("preserves header tags (h1-h6)", () => {
		const html = "<h1>Title</h1><h2>Subtitle</h2><h3>Section</h3>";
		const result = sanitizeHtml(html);
		expect(result).toContain("<h1>");
		expect(result).toContain("<h2>");
		expect(result).toContain("<h3>");
	});

	it("preserves list tags (ul, ol, li)", () => {
		const html = "<ul><li>Item 1</li><li>Item 2</li></ul><ol><li>Num 1</li></ol>";
		const result = sanitizeHtml(html);
		expect(result).toContain("<ul>");
		expect(result).toContain("<ol>");
		expect(result).toContain("<li>");
	});

	it("preserves code blocks (pre, code)", () => {
		const html = '<pre><code>const x = 1;</code></pre>';
		const result = sanitizeHtml(html);
		expect(result).toContain("<pre>");
		expect(result).toContain("<code>");
		expect(result).toContain("const x = 1;");
	});

	it("preserves links with href attribute", () => {
		const html = '<a href="https://example.com">Link</a>';
		const result = sanitizeHtml(html);
		expect(result).toContain("<a");
		expect(result).toContain('href="https://example.com"');
		expect(result).toContain("Link");
	});

	it("adds target=_blank and rel=noopener noreferrer to links", () => {
		const html = '<a href="https://example.com">Link</a>';
		const result = sanitizeHtml(html);
		expect(result).toContain('target="_blank"');
		expect(result).toContain('rel="noopener noreferrer"');
	});

	it("overwrites existing target and rel attributes on links", () => {
		const html = '<a href="https://example.com" target="_self" rel="author">Link</a>';
		const result = sanitizeHtml(html);
		expect(result).toContain('target="_blank"');
		expect(result).toContain('rel="noopener noreferrer"');
		expect(result).not.toContain('target="_self"');
		expect(result).not.toContain('rel="author"');
	});

	it("removes img tags for security (XSS via onerror/onload)", () => {
		// img tags are removed because:
		// 1. Alt doesn't fetch/display images anyway
		// 2. img tags are a major XSS vector (onerror, onload events)
		const html = '<img src="https://example.com/img.jpg" alt="Test image" onerror="alert(1)">';
		const result = sanitizeHtml(html);
		expect(result).not.toContain("<img");
		expect(result).not.toContain("onerror");
	});

	it("preserves text formatting (strong, em, b, i, u)", () => {
		const html = "<strong>bold</strong><em>italic</em><b>b</b><i>i</i><u>underline</u>";
		const result = sanitizeHtml(html);
		expect(result).toContain("<strong>");
		expect(result).toContain("<em>");
		expect(result).toContain("<b>");
		expect(result).toContain("<i>");
		expect(result).toContain("<u>");
	});

	it("preserves blockquote tags", () => {
		const html = "<blockquote>Quoted text</blockquote>";
		const result = sanitizeHtml(html);
		expect(result).toContain("<blockquote>");
		expect(result).toContain("Quoted text");
	});

	it("preserves table tags", () => {
		const html = "<table><thead><tr><th>Header</th></tr></thead><tbody><tr><td>Cell</td></tr></tbody></table>";
		const result = sanitizeHtml(html);
		expect(result).toContain("<table>");
		expect(result).toContain("<th>");
		expect(result).toContain("<td>");
	});

	it("preserves structural elements (div, span, article, section)", () => {
		const html = "<div><article><section><span>Content</span></section></article></div>";
		const result = sanitizeHtml(html);
		expect(result).toContain("<div>");
		expect(result).toContain("<article>");
		expect(result).toContain("<section>");
		expect(result).toContain("<span>");
	});

	it("removes script tags", () => {
		const html = "<p>Safe</p><script>alert('XSS')</script>";
		const result = sanitizeHtml(html);
		expect(result).not.toContain("<script");
		expect(result).not.toContain("alert");
		expect(result).toContain("Safe");
	});

	it("removes onclick and other event handlers", () => {
		const html = '<p onclick="alert(\'XSS\')">Text</p>';
		const result = sanitizeHtml(html);
		expect(result).not.toContain("onclick");
		expect(result).toContain("Text");
	});

	it("removes javascript: URLs", () => {
		const html = '<a href="javascript:alert(\'XSS\')">Click</a>';
		const result = sanitizeHtml(html);
		expect(result).not.toContain("javascript:");
	});

	it("removes style tags", () => {
		const html = "<style>.evil { display: none; }</style><p>Content</p>";
		const result = sanitizeHtml(html);
		expect(result).not.toContain("<style");
		expect(result).not.toContain(".evil");
	});

	it("removes iframe tags", () => {
		const html = '<iframe src="https://evil.com"></iframe><p>Safe</p>';
		const result = sanitizeHtml(html);
		expect(result).not.toContain("<iframe");
		expect(result).toContain("Safe");
	});

	it("removes form tags", () => {
		const html = '<form action="https://evil.com"><input type="submit"></form>';
		const result = sanitizeHtml(html);
		expect(result).not.toContain("<form");
		expect(result).not.toContain("<input");
	});

	it("removes inline SVG with scripts (XSS vector)", () => {
		// Inline SVG with onload IS dangerous and should be removed
		const html = '<svg onload="alert(1)"><circle r="50"/></svg><p>Safe</p>';
		const result = sanitizeHtml(html);
		// SVG tags should be removed entirely
		expect(result).not.toContain("<svg");
		expect(result).toContain("Safe");
	});

	it("removes all img tags including those with data: URLs", () => {
		// All img tags are removed for security - onerror/onload are XSS vectors
		const html = '<img src="data:image/png;base64,iVBOR..." alt="test"><p>Text</p>';
		const result = sanitizeHtml(html);
		expect(result).not.toContain("<img");
		expect(result).toContain("Text");
	});
});

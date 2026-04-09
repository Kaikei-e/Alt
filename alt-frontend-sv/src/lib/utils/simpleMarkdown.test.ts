import { describe, expect, it } from "vitest";
import { parseMarkdown } from "./simpleMarkdown";

describe("parseMarkdown", () => {
	describe("edge cases", () => {
		it("returns empty string for empty input", () => {
			expect(parseMarkdown("")).toBe("");
		});

		it("returns empty string for null input", () => {
			// @ts-expect-error Testing null input
			expect(parseMarkdown(null)).toBe("");
		});

		it("returns empty string for undefined input", () => {
			// @ts-expect-error Testing undefined input
			expect(parseMarkdown(undefined)).toBe("");
		});
	});

	describe("headers", () => {
		it("converts # to h1 with correct classes", () => {
			const result = parseMarkdown("# Hello World");
			expect(result).toContain("<h1");
			expect(result).toContain("Hello World");
			expect(result).toContain("text-xl");
			expect(result).toContain("font-bold");
		});

		it("converts ## to h2 with correct classes", () => {
			const result = parseMarkdown("## Section Title");
			expect(result).toContain("<h2");
			expect(result).toContain("Section Title");
			expect(result).toContain("text-lg");
			expect(result).toContain("font-semibold");
		});

		it("converts ### to h3 with correct classes", () => {
			const result = parseMarkdown("### Subsection");
			expect(result).toContain("<h3");
			expect(result).toContain("Subsection");
			expect(result).toContain("text-md");
		});

		it("handles headers with inline formatting", () => {
			const result = parseMarkdown("# **Bold** Header");
			expect(result).toContain("<h1");
			expect(result).toContain("<strong>Bold</strong>");
		});
	});

	describe("inline formatting", () => {
		it("converts **text** to <strong>", () => {
			const result = parseMarkdown("This is **bold** text");
			expect(result).toContain("<strong>bold</strong>");
		});

		it("converts *text* to <em>", () => {
			const result = parseMarkdown("This is *italic* text");
			expect(result).toContain("<em>italic</em>");
		});

		it("converts `code` to inline code with classes", () => {
			const result = parseMarkdown("Use `const` for constants");
			expect(result).toContain("<code");
			expect(result).toContain("const");
			expect(result).toContain("bg-muted");
		});

		it("handles multiple inline formats in same line", () => {
			const result = parseMarkdown("**bold** and *italic* and `code`");
			expect(result).toContain("<strong>bold</strong>");
			expect(result).toContain("<em>italic</em>");
			expect(result).toContain("<code");
		});
	});

	describe("unordered lists", () => {
		it("converts - items to <ul>", () => {
			const result = parseMarkdown("- Item 1\n- Item 2\n- Item 3");
			expect(result).toContain("<ul");
			expect(result).toContain("<li>Item 1</li>");
			expect(result).toContain("<li>Item 2</li>");
			expect(result).toContain("<li>Item 3</li>");
			expect(result).toContain("list-disc");
		});

		it("handles list item with inline formatting", () => {
			const result = parseMarkdown("- **Bold** item\n- *Italic* item");
			expect(result).toContain("<li><strong>Bold</strong> item</li>");
			expect(result).toContain("<li><em>Italic</em> item</li>");
		});
	});

	describe("ordered lists", () => {
		it("converts 1. items to <ol>", () => {
			const result = parseMarkdown("1. First\n2. Second\n3. Third");
			expect(result).toContain("<ol");
			expect(result).toContain("<li>First</li>");
			expect(result).toContain("<li>Second</li>");
			expect(result).toContain("<li>Third</li>");
			expect(result).toContain("list-decimal");
		});
	});

	describe("list transitions", () => {
		it("handles list followed by paragraph", () => {
			const result = parseMarkdown(
				"- Item 1\n- Item 2\n\nParagraph after list",
			);
			expect(result).toContain("</ul>");
			expect(result).toContain("<p");
			expect(result).toContain("Paragraph after list");
		});

		it("handles switching between list types", () => {
			const result = parseMarkdown(
				"- Unordered\n\n1. Ordered\n2. Second ordered",
			);
			expect(result).toContain("<ul");
			expect(result).toContain("</ul>");
			expect(result).toContain("<ol");
			expect(result).toContain("</ol>");
		});
	});

	describe("code blocks", () => {
		it("converts ``` to pre/code", () => {
			const result = parseMarkdown("```\nconst x = 1;\n```");
			expect(result).toContain("<pre>");
			expect(result).toContain("<code");
			expect(result).toContain("const x = 1;");
		});

		it("escapes HTML in code blocks", () => {
			const result = parseMarkdown("```\n<div>test</div>\n```");
			expect(result).toContain("&lt;div&gt;");
			expect(result).toContain("&lt;/div&gt;");
			expect(result).not.toContain("<div>");
		});

		it("handles code block with whitespace-pre-wrap class", () => {
			const result = parseMarkdown("```\ncode\n```");
			expect(result).toContain("whitespace-pre-wrap");
		});
	});

	describe("paragraphs", () => {
		it("wraps plain text in <p> tags", () => {
			const result = parseMarkdown("This is plain text.");
			expect(result).toContain("<p");
			expect(result).toContain("This is plain text.");
			expect(result).toContain("</p>");
		});

		it("separates paragraphs on empty lines", () => {
			const result = parseMarkdown("First paragraph.\n\nSecond paragraph.");
			const pCount = (result.match(/<p/g) || []).length;
			expect(pCount).toBe(2);
		});

		it("adds proper paragraph styling", () => {
			const result = parseMarkdown("Text with styling");
			expect(result).toContain("mb-2");
			expect(result).toContain("leading-relaxed");
		});
	});

	describe("literal escape sequences", () => {
		it("converts literal \\n to actual newlines", () => {
			// This simulates the GPT-OSS model issue where literal \n appears in output
			const result = parseMarkdown("## Heading\\n\\nParagraph text");
			expect(result).toContain("<h2");
			expect(result).toContain("Heading");
			expect(result).toContain("<p");
			expect(result).toContain("Paragraph text");
		});

		it("handles markdown with literal \\n in lists", () => {
			const result = parseMarkdown("- Item 1\\n- Item 2\\n- Item 3");
			expect(result).toContain("<ul");
			expect(result).toContain("<li>Item 1</li>");
			expect(result).toContain("<li>Item 2</li>");
			expect(result).toContain("<li>Item 3</li>");
		});
	});

	describe("XSS prevention", () => {
		it("escapes script tags in inline text", () => {
			const result = parseMarkdown('<script>alert("xss")</script>');
			expect(result).not.toContain("<script>");
			expect(result).toContain("&lt;script&gt;");
		});

		it("escapes img onerror XSS vectors", () => {
			const result = parseMarkdown('<img onerror="alert(1)" src=x>');
			expect(result).not.toContain("<img");
			expect(result).toContain("&lt;img");
		});

		it("escapes event handlers in tags", () => {
			const result = parseMarkdown(
				'<div onmouseover="alert(1)">hover me</div>',
			);
			// The tag itself is escaped so onmouseover cannot execute
			expect(result).not.toContain("<div onmouseover");
			expect(result).toContain("&lt;div");
		});

		it("escapes HTML in header content", () => {
			const result = parseMarkdown('## <script>alert("xss")</script>');
			expect(result).toContain("<h2");
			expect(result).not.toContain("<script>");
			expect(result).toContain("&lt;script&gt;");
		});

		it("escapes HTML in list items", () => {
			const result = parseMarkdown("- <img src=x onerror=alert(1)>");
			expect(result).toContain("<li>");
			expect(result).not.toContain("<img");
		});

		it("preserves markdown formatting while escaping HTML", () => {
			const result = parseMarkdown("**bold** and <script>xss</script>");
			expect(result).toContain("<strong>bold</strong>");
			expect(result).not.toContain("<script>");
		});
	});

	describe("horizontal rules", () => {
		it("converts --- to <hr>", () => {
			const result = parseMarkdown("---");
			expect(result).toContain("<hr");
		});

		it("converts *** to <hr>", () => {
			const result = parseMarkdown("***");
			expect(result).toContain("<hr");
		});

		it("converts ___ to <hr>", () => {
			const result = parseMarkdown("___");
			expect(result).toContain("<hr");
		});

		it("does not convert -- (too few) to <hr>", () => {
			const result = parseMarkdown("--");
			expect(result).not.toContain("<hr");
		});

		it("handles hr between paragraphs", () => {
			const result = parseMarkdown("Above\n\n---\n\nBelow");
			expect(result).toContain("<hr");
			expect(result).toContain("Above");
			expect(result).toContain("Below");
			const hrPos = result.indexOf("<hr");
			const abovePos = result.indexOf("Above");
			const belowPos = result.indexOf("Below");
			expect(abovePos).toBeLessThan(hrPos);
			expect(hrPos).toBeLessThan(belowPos);
		});
	});

	describe("blockquotes", () => {
		it("converts > text to <blockquote>", () => {
			const result = parseMarkdown("> This is a quote");
			expect(result).toContain("<blockquote");
			expect(result).toContain("This is a quote");
		});

		it("handles multi-line blockquote", () => {
			const result = parseMarkdown("> Line one\n> Line two");
			expect(result).toContain("<blockquote");
			expect(result).toContain("Line one");
			expect(result).toContain("Line two");
			// Should be a single blockquote
			const count = (result.match(/<blockquote/g) || []).length;
			expect(count).toBe(1);
		});

		it("handles blockquote with inline formatting", () => {
			const result = parseMarkdown("> This is **bold** in a quote");
			expect(result).toContain("<blockquote");
			expect(result).toContain("<strong>bold</strong>");
		});

		it("handles blockquote followed by paragraph", () => {
			const result = parseMarkdown("> A quote\n\nA paragraph");
			expect(result).toContain("<blockquote");
			expect(result).toContain("</blockquote>");
			expect(result).toContain("<p");
			expect(result).toContain("A paragraph");
		});

		it("escapes HTML in blockquotes", () => {
			const result = parseMarkdown('> <script>alert("xss")</script>');
			expect(result).toContain("<blockquote");
			expect(result).not.toContain("<script>");
			expect(result).toContain("&lt;script&gt;");
		});
	});

	describe("links", () => {
		it("converts [text](url) to <a> tag", () => {
			const result = parseMarkdown(
				"Visit [Example](https://example.com) today",
			);
			expect(result).toContain('<a href="https://example.com"');
			expect(result).toContain(">Example</a>");
		});

		it("link opens in new tab with rel=noopener", () => {
			const result = parseMarkdown("[Link](https://example.com)");
			expect(result).toContain('target="_blank"');
			expect(result).toContain('rel="noopener noreferrer"');
		});

		it("handles multiple links in one line", () => {
			const result = parseMarkdown(
				"[A](https://example.com/a) and [B](https://example.com/b)",
			);
			const linkCount = (result.match(/<a href/g) || []).length;
			expect(linkCount).toBe(2);
		});

		it("handles link inside bold text", () => {
			const result = parseMarkdown("**[Bold Link](https://example.com)**");
			expect(result).toContain("<strong>");
			expect(result).toContain('<a href="https://example.com"');
		});

		it("does not convert javascript: URLs", () => {
			const result = parseMarkdown("[click](javascript:alert(1))");
			expect(result).not.toContain("<a href");
		});

		it("does not convert data: URLs", () => {
			const result = parseMarkdown(
				"[click](data:text/html,<script>alert(1)</script>)",
			);
			expect(result).not.toContain("<a href");
		});
	});

	describe("complex content", () => {
		it("handles mixed content types", () => {
			const markdown = `# Title

This is a paragraph with **bold** text.

- List item 1
- List item 2

\`\`\`
code block
\`\`\`

## Another Section

Final paragraph.`;

			const result = parseMarkdown(markdown);
			expect(result).toContain("<h1");
			expect(result).toContain("<p");
			expect(result).toContain("<strong>bold</strong>");
			expect(result).toContain("<ul");
			expect(result).toContain("<pre>");
			expect(result).toContain("<h2");
		});

		it("preserves content order", () => {
			const markdown = "# First\n\nMiddle\n\n## Last";
			const result = parseMarkdown(markdown);
			const h1Pos = result.indexOf("<h1");
			const pPos = result.indexOf("<p");
			const h2Pos = result.indexOf("<h2");
			expect(h1Pos).toBeLessThan(pPos);
			expect(pPos).toBeLessThan(h2Pos);
		});

		it("handles report-like content with all features", () => {
			const markdown = `## AI技術の最新動向

近年、**大規模言語モデル**（LLM）の進化が加速しています。

> GPT-4やGeminiの登場により、自然言語処理の精度は飛躍的に向上しました。

### 主要な進展

- *Transformer*アーキテクチャの改良
- マルチモーダル対応の拡大
- 推論コストの低減

---

詳細は[公式サイト](https://example.com)を参照してください。`;

			const result = parseMarkdown(markdown);
			expect(result).toContain("<h2");
			expect(result).toContain("<strong>大規模言語モデル</strong>");
			expect(result).toContain("<blockquote");
			expect(result).toContain("<h3");
			expect(result).toContain("<ul");
			expect(result).toContain("<em>Transformer</em>");
			expect(result).toContain("<hr");
			expect(result).toContain('<a href="https://example.com"');
		});
	});
});

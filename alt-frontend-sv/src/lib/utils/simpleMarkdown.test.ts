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
			const result = parseMarkdown("- Item 1\n- Item 2\n\nParagraph after list");
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
	});
});

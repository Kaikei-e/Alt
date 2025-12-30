import { describe, expect, it } from "vitest";
import {
	canonicalize,
	formatPublishedDate,
	generateExcerptFromDescription,
	mergeTagsLabel,
	normalizeUrl,
} from "./feed";

describe("formatPublishedDate", () => {
	it("formats ISO date string correctly", () => {
		const result = formatPublishedDate("2025-11-23T10:30:00Z");
		expect(result).toBe("Nov 23, 2025");
	});

	it("formats date without time correctly", () => {
		const result = formatPublishedDate("2024-01-15");
		expect(result).toBe("Jan 15, 2024");
	});

	it("returns empty string for null", () => {
		expect(formatPublishedDate(null)).toBe("");
	});

	it("returns empty string for undefined", () => {
		expect(formatPublishedDate(undefined)).toBe("");
	});

	it("returns empty string for empty string", () => {
		expect(formatPublishedDate("")).toBe("");
	});

	it("returns empty string for invalid date", () => {
		expect(formatPublishedDate("not-a-date")).toBe("");
	});
});

describe("mergeTagsLabel", () => {
	it("joins tags with ' / ' separator", () => {
		const result = mergeTagsLabel(["Next.js", "Performance", "React"]);
		expect(result).toBe("Next.js / Performance / React");
	});

	it("returns single tag without separator", () => {
		const result = mergeTagsLabel(["TypeScript"]);
		expect(result).toBe("TypeScript");
	});

	it("returns empty string for empty array", () => {
		expect(mergeTagsLabel([])).toBe("");
	});

	it("returns empty string for null", () => {
		expect(mergeTagsLabel(null)).toBe("");
	});

	it("returns empty string for undefined", () => {
		expect(mergeTagsLabel(undefined)).toBe("");
	});
});

describe("normalizeUrl", () => {
	it("removes UTM parameters", () => {
		const url =
			"https://example.com/article?utm_source=twitter&utm_medium=social";
		const result = normalizeUrl(url);
		expect(result).toBe("https://example.com/article");
	});

	it("removes fbclid parameter", () => {
		const url = "https://example.com/post?fbclid=abc123";
		const result = normalizeUrl(url);
		expect(result).toBe("https://example.com/post");
	});

	it("removes gclid parameter", () => {
		const url = "https://example.com/page?gclid=xyz789";
		const result = normalizeUrl(url);
		expect(result).toBe("https://example.com/page");
	});

	it("removes msclkid parameter", () => {
		const url = "https://example.com/page?msclkid=click123";
		const result = normalizeUrl(url);
		expect(result).toBe("https://example.com/page");
	});

	it("removes fragment/hash", () => {
		const url = "https://example.com/page#section";
		const result = normalizeUrl(url);
		expect(result).toBe("https://example.com/page");
	});

	it("removes trailing slash from path", () => {
		const url = "https://example.com/blog/post/";
		const result = normalizeUrl(url);
		expect(result).toBe("https://example.com/blog/post");
	});

	it("keeps trailing slash for root path", () => {
		const url = "https://example.com/";
		const result = normalizeUrl(url);
		expect(result).toBe("https://example.com/");
	});

	it("preserves non-tracking query params", () => {
		const url = "https://example.com/search?q=test&page=2";
		const result = normalizeUrl(url);
		expect(result).toBe("https://example.com/search?q=test&page=2");
	});

	it("removes only tracking params, keeps others", () => {
		const url =
			"https://example.com/article?id=123&utm_source=twitter&ref=home";
		const result = normalizeUrl(url);
		expect(result).toBe("https://example.com/article?id=123&ref=home");
	});

	it("returns empty string for null", () => {
		expect(normalizeUrl(null)).toBe("");
	});

	it("returns empty string for undefined", () => {
		expect(normalizeUrl(undefined)).toBe("");
	});

	it("returns empty string for empty string", () => {
		expect(normalizeUrl("")).toBe("");
	});

	it("returns original URL on parse failure", () => {
		const invalidUrl = "not-a-valid-url";
		expect(normalizeUrl(invalidUrl)).toBe(invalidUrl);
	});
});

describe("generateExcerptFromDescription", () => {
	it("returns full text if under maxLength", () => {
		const text = "Short description";
		expect(generateExcerptFromDescription(text)).toBe("Short description");
	});

	it("truncates at word boundary with ellipsis", () => {
		const text =
			"This is a very long description that should be truncated at a word boundary to ensure readability and maintain proper formatting for the excerpt display.";
		const result = generateExcerptFromDescription(text, 50);
		expect(result).toMatch(/\.\.\.$/);
		expect(result.length).toBeLessThanOrEqual(53); // 50 + "..."
	});

	it("strips HTML tags", () => {
		const html = "<p>Hello <strong>world</strong></p>";
		expect(generateExcerptFromDescription(html)).toBe("Hello world");
	});

	it("normalizes whitespace", () => {
		const text = "Multiple   spaces   and\n\nnewlines";
		expect(generateExcerptFromDescription(text)).toBe(
			"Multiple spaces and newlines",
		);
	});

	it("returns empty string for null", () => {
		expect(generateExcerptFromDescription(null)).toBe("");
	});

	it("returns empty string for undefined", () => {
		expect(generateExcerptFromDescription(undefined)).toBe("");
	});

	it("returns empty string for empty string", () => {
		expect(generateExcerptFromDescription("")).toBe("");
	});

	it("respects custom maxLength parameter", () => {
		const text = "A short text";
		expect(generateExcerptFromDescription(text, 5).length).toBeLessThanOrEqual(
			8,
		); // 5 + "..."
	});
});

describe("canonicalize", () => {
	it("delegates to normalizeUrl", () => {
		const url = "https://example.com/page?utm_source=test#hash";
		expect(canonicalize(url)).toBe(normalizeUrl(url));
	});
});

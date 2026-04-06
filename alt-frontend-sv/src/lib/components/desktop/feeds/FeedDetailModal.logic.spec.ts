/**
 * FeedDetailModal — article content response processing logic
 *
 * Regression test for infinite re-fetch loop:
 * When getFeedContentOnTheFlyClient returns empty content, processArticleFetchResponse
 * must set contentError to break the auto-fetch $effect loop.
 *
 * Without this fix, the auto-fetch $effect re-fires infinitely because:
 * - articleContent stays null (empty string is falsy)
 * - contentError stays null (no error thrown)
 * - isFetchingContent resets to false
 * → all conditions for auto-fetch remain true → infinite loop
 */
import { describe, expect, it } from "vitest";
import { processArticleFetchResponse } from "./FeedDetailModal.logic";

describe("processArticleFetchResponse", () => {
	it("returns content and articleID when content is non-empty", () => {
		const response = {
			content: "<p>Full article content.</p>",
			article_id: "article-123",
			og_image_url: "",
			og_image_proxy_url: "",
		};

		const result = processArticleFetchResponse(response);

		expect(result.articleContent).toBe("<p>Full article content.</p>");
		expect(result.articleID).toBe("article-123");
		expect(result.contentError).toBeNull();
	});

	it("sets contentError when content is empty string", () => {
		const response = {
			content: "",
			article_id: "",
			og_image_url: "",
			og_image_proxy_url: "",
		};

		const result = processArticleFetchResponse(response);

		expect(result.articleContent).toBeNull();
		expect(result.articleID).toBeNull();
		expect(result.contentError).toBe("Article content could not be retrieved");
	});

	it("sets contentError when content is whitespace-only", () => {
		const response = {
			content: "   ",
			article_id: "article-456",
			og_image_url: "",
			og_image_proxy_url: "",
		};

		const result = processArticleFetchResponse(response);

		expect(result.articleContent).toBeNull();
		expect(result.contentError).toBe("Article content could not be retrieved");
	});

	it("returns articleID as null when article_id is empty", () => {
		const response = {
			content: "<p>Content</p>",
			article_id: "",
			og_image_url: "",
			og_image_proxy_url: "",
		};

		const result = processArticleFetchResponse(response);

		expect(result.articleContent).toBe("<p>Content</p>");
		expect(result.articleID).toBeNull();
		expect(result.contentError).toBeNull();
	});
});

/**
 * Article API Contract Tests
 *
 * Validates article-related proto schema conformance.
 */
import { describe, it, expect } from "vitest";
import { create, toBinary, fromBinary } from "@bufbuild/protobuf";
import {
	FetchArticleContentResponseSchema,
} from "$lib/gen/alt/articles/v2/articles_pb";
import { buildConnectArticleContent } from "../../../tests/e2e/fixtures/factories";

describe("Article API Contract", () => {
	it("FetchArticleContentResponse conforms to proto schema", () => {
		const mockData = buildConnectArticleContent();
		const response = create(FetchArticleContentResponseSchema, {
			url: mockData.url,
			content: mockData.content,
			articleId: mockData.articleId,
		});

		expect(response.url).toBe(mockData.url);
		expect(response.content).toContain("Mocked article content");
		expect(response.articleId).toBe(mockData.articleId);
	});

	it("round-trips through proto serialization", () => {
		const original = create(FetchArticleContentResponseSchema, {
			url: "https://example.com/article",
			content: "<p>Article body</p>",
			articleId: "art-123",
		});

		const binary = toBinary(FetchArticleContentResponseSchema, original);
		const deserialized = fromBinary(
			FetchArticleContentResponseSchema,
			binary,
		);

		expect(deserialized.url).toBe(original.url);
		expect(deserialized.content).toBe(original.content);
		expect(deserialized.articleId).toBe(original.articleId);
	});
});

import { beforeEach, describe, expect, it, vi } from "vitest";
import { ArticleApi } from "@/lib/api/articles/ArticleApi";
import type { Article } from "@/schema/article";

const createMockArticle = (id: string, overrides: Partial<Article> = {}): Article => ({
  id,
  title: `Test Article ${id}`,
  content: `This is test content ${id}`,
  url: `https://example.com/articles/${id}`,
  published_at: "2024-01-01T00:00:00.000Z",
  ...overrides,
});

describe("ArticleApi.searchArticles", () => {
  let mockApiClient: any;
  let articleApi: ArticleApi;

  beforeEach(() => {
    mockApiClient = {
      get: vi.fn(),
      post: vi.fn(),
      clearCache: vi.fn(),
    } as any;

    articleApi = new ArticleApi(mockApiClient);
  });

  it("should return backend response with lowercase fields directly", async () => {
    const mockBackendResponse: Article[] = [createMockArticle("1"), createMockArticle("2")];

    mockApiClient.get.mockResolvedValueOnce(mockBackendResponse);

    const result = await articleApi.searchArticles("test query");

    expect(mockApiClient.get).toHaveBeenCalledWith("/v1/articles/search?q=test query");

    expect(result).toEqual([createMockArticle("1"), createMockArticle("2")]);
  });

  it("should handle empty results", async () => {
    mockApiClient.get.mockResolvedValueOnce([]);

    const result = await articleApi.searchArticles("nonexistent");

    expect(result).toEqual([]);
  });

  it("should handle single result", async () => {
    const mockBackendResponse: Article[] = [
      createMockArticle("single", {
        title: "Single Article",
        content: "Single content",
      }),
    ];

    mockApiClient.get.mockResolvedValueOnce(mockBackendResponse);

    const result = await articleApi.searchArticles("single");

    expect(result).toHaveLength(1);
    expect(result[0]).toEqual(
      createMockArticle("single", {
        title: "Single Article",
        content: "Single content",
      })
    );
  });

  it("should properly encode query parameters", async () => {
    mockApiClient.get.mockResolvedValueOnce([]);

    await articleApi.searchArticles("test query with spaces");

    expect(mockApiClient.get).toHaveBeenCalledWith("/v1/articles/search?q=test query with spaces");
  });

  it("should handle backend errors", async () => {
    const error = new Error("Backend error");
    mockApiClient.get.mockRejectedValueOnce(error);

    await expect(articleApi.searchArticles("test")).rejects.toThrow("Backend error");
  });

  it("should correctly pass through all fields from backend response", async () => {
    const mockBackendResponse: Article[] = [
      createMockArticle("test-id-123", {
        title: "Article Title with Special Chars: !@#$%",
        content: "Content with\nmultiple\nlines",
      }),
    ];

    mockApiClient.get.mockResolvedValueOnce(mockBackendResponse);

    const result = await articleApi.searchArticles("special");

    expect(result[0].id).toBe("test-id-123");
    expect(result[0].title).toBe("Article Title with Special Chars: !@#$%");
    expect(result[0].content).toBe("Content with\nmultiple\nlines");
  });
});

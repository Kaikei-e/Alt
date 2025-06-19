import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { feedsApi } from "@/lib/api";
import { BackendFeedItem } from "@/schema/feed";

// Mock fetch for integration testing
const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

describe("Feeds API Integration", () => {
  beforeEach(() => {
    mockFetch.mockClear();
    feedsApi.clearCache();
  });

  afterEach(() => {
    mockFetch.mockReset();
  });

  describe("getFeeds", () => {
    it("should fetch and transform feed data correctly", async () => {
      const mockBackendData: BackendFeedItem[] = [
        {
          title: "Tech News",
          description: "Latest technology news",
          link: "https://example.com/feed1",
          published: "2023-01-01",
        },
        {
          title: "Dev Blog",
          description: "Development insights",
          link: "https://example.com/feed2",
          published: "2023-01-02",
        },
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockBackendData),
      });

      const result = await feedsApi.getFeeds(1, 10);

      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost/api/v1/feeds/fetch/limit/10",
        expect.objectContaining({
          method: "GET",
        }),
      );

      expect(result).toEqual([
        {
          id: "https://example.com/feed1",
          title: "Tech News",
          description: "Latest technology news",
          link: "https://example.com/feed1",
          published: "2023-01-01",
        },
        {
          id: "https://example.com/feed2",
          title: "Dev Blog",
          description: "Development insights",
          link: "https://example.com/feed2",
          published: "2023-01-02",
        },
      ]);
    });

    it("should handle API errors gracefully", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      await expect(feedsApi.getFeeds()).rejects.toThrow();
    });

    it("should handle empty response", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve([]),
      });

      const result = await feedsApi.getFeeds();
      expect(result).toEqual([]);
    });
  });

  describe("searchFeeds", () => {
    it("should search feeds and return results", async () => {
      const mockSearchResults: BackendFeedItem[] = [
        {
          title: "JavaScript News",
          description: "Latest JS updates",
          link: "https://js-news.com",
          published: "2023-01-01",
        },
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockSearchResults),
      });

      const result = await feedsApi.searchFeeds("javascript");

      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost/api/v1/feeds/search",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ query: "javascript" }),
        }),
      );

      expect(result).toEqual({
        results: mockSearchResults,
        error: null,
      });
    });

    it("should handle search errors", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Search failed"));

      const result = await feedsApi.searchFeeds("test");

      expect(result).toEqual({
        results: [],
        error: "Search failed",
      });
    });
  });

  describe("registerRssFeed", () => {
    it("should register a new RSS feed", async () => {
      const mockResponse = { message: "Feed registered successfully" };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      });

      const result = await feedsApi.registerRssFeed("https://example.com/rss");

      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost/api/v1/rss-feed-link/register",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ url: "https://example.com/rss" }),
        }),
      );

      expect(result).toEqual(mockResponse);
    });
  });

  describe("checkHealth", () => {
    it("should check API health", async () => {
      const mockHealthResponse = { status: "healthy" };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockHealthResponse),
      });

      const result = await feedsApi.checkHealth();

      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost/api/v1/health",
        expect.objectContaining({
          method: "GET",
        }),
      );

      expect(result).toEqual(mockHealthResponse);
    });
  });

  describe("caching behavior", () => {
    it("should cache repeated requests", async () => {
      const mockResponse = { status: "healthy" };

      mockFetch.mockResolvedValue({
        ok: true,
        json: () => Promise.resolve(mockResponse),
      });

      // Make the same request twice
      await feedsApi.checkHealth();
      await feedsApi.checkHealth();

      // Should only make one actual HTTP request due to caching
      expect(mockFetch).toHaveBeenCalledTimes(1);
    });

    it("should invalidate cache after POST requests", async () => {
      const mockGetResponse = { status: "healthy" };
      const mockPostResponse = { message: "success" };

      // First GET request
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockGetResponse),
      });

      await feedsApi.checkHealth();
      expect(mockFetch).toHaveBeenCalledTimes(1);

      // POST request should invalidate cache
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockPostResponse),
      });

      await feedsApi.registerRssFeed("https://example.com/rss");

      // Second GET request should make new HTTP call
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.resolve(mockGetResponse),
      });

      await feedsApi.checkHealth();
      expect(mockFetch).toHaveBeenCalledTimes(3);
    });
  });

  describe("error handling", () => {
    it("should handle HTTP error responses", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
      });

      await expect(feedsApi.checkHealth()).rejects.toThrow();
    });

    it("should handle network errors", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      await expect(feedsApi.checkHealth()).rejects.toThrow("Network error");
    });

    it("should handle malformed JSON responses", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: () => Promise.reject(new Error("Invalid JSON")),
      });

      await expect(feedsApi.checkHealth()).rejects.toThrow("Invalid JSON");
    });
  });
});

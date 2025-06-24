import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { apiClient, feedsApi } from "@/lib/api";
import { BackendFeedItem, Feed } from "@/schema/feed";

// Mock fetch globally
const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

describe("ApiClient", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiClient.clearCache();
    vi.useFakeTimers();
    // Reset fetch mock
    mockFetch.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
    mockFetch.mockReset();
  });

  describe("get method", () => {
    it("should make GET request with correct headers", async () => {
      const mockResponse = { data: "test" };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResponse),
      });

      const result = await apiClient.get("/test");

      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost/api/test",
        expect.objectContaining({
          method: "GET",
          headers: expect.objectContaining({
            "Content-Type": "application/json",
            "Cache-Control": "max-age=300",
            "Accept-Encoding": "gzip, deflate, br",
          }),
          keepalive: true,
        }),
      );
      expect(result).toEqual(mockResponse);
    });

    it("should cache successful responses", async () => {
      const mockResponse = { data: "test" };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResponse),
      });

      // First call
      const result1 = await apiClient.get("/test");
      // Second call should use cache
      const result2 = await apiClient.get("/test");

      expect(mockFetch).toHaveBeenCalledTimes(1);
      expect(result1).toEqual(mockResponse);
      expect(result2).toEqual(mockResponse);
    });

    it("should handle cache expiration", async () => {
      const mockResponse1 = { data: "test1" };
      const mockResponse2 = { data: "test2" };

      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: vi.fn().mockResolvedValue(mockResponse1),
        })
        .mockResolvedValueOnce({
          ok: true,
          json: vi.fn().mockResolvedValue(mockResponse2),
        });

      // First call
      const result1 = await apiClient.get("/test", 0.001); // Very short TTL

      // Advance time beyond TTL
      vi.advanceTimersByTime(100);

      // Second call should make new request
      const result2 = await apiClient.get("/test", 0.001);

      expect(mockFetch).toHaveBeenCalledTimes(2);
      expect(result1).toEqual(mockResponse1);
      expect(result2).toEqual(mockResponse2);
    });

    it("should throw error for failed requests", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 404,
        statusText: "Not Found",
      });

      await expect(apiClient.get("/test")).rejects.toThrow(
        "API request failed: 404 Not Found",
      );
    });

    it("should deduplicate concurrent requests", async () => {
      const mockResponse = { data: "test" };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResponse),
      });

      // Make concurrent requests
      const [result1, result2, result3] = await Promise.all([
        apiClient.get("/test"),
        apiClient.get("/test"),
        apiClient.get("/test"),
      ]);

      expect(mockFetch).toHaveBeenCalledTimes(1);
      expect(result1).toEqual(mockResponse);
      expect(result2).toEqual(mockResponse);
      expect(result3).toEqual(mockResponse);
    });
  });

  describe("post method", () => {
    it("should make POST request with correct headers and body", async () => {
      const mockResponse = { success: true };
      const postData = { key: "value" };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResponse),
      });

      const result = await apiClient.post("/test", postData);

      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost/api/test",
        expect.objectContaining({
          method: "POST",
          headers: expect.objectContaining({
            "Content-Type": "application/json",
            "Accept-Encoding": "gzip, deflate, br",
          }),
          body: JSON.stringify(postData),
          keepalive: true,
        }),
      );
      expect(result).toEqual(mockResponse);
    });

    it("should invalidate cache after POST", async () => {
      // Setup cache with GET request
      const getCacheResponse = { data: "cached" };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => getCacheResponse,
      });

      await apiClient.get("/test");
      expect(mockFetch).toHaveBeenCalledTimes(1);

      // POST request should invalidate cache
      const postResponse = { success: true };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => postResponse,
      });

      await apiClient.post("/test", { data: "new" });

      // Subsequent GET should make new request
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => ({ data: "new_cached" }),
      });

      await apiClient.get("/test");

      expect(mockFetch).toHaveBeenCalledTimes(3);
    });

    it("should handle API errors in response", async () => {
      const errorResponse = { error: "Something went wrong" };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: async () => errorResponse,
      });

      await expect(apiClient.post("/test", {})).rejects.toThrow(
        "Something went wrong",
      );
    });

    it("should throw error for failed POST requests", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
      });

      await expect(apiClient.post("/test", {})).rejects.toThrow(
        "API request failed: 500 Internal Server Error",
      );
    });
  });

  describe("clearCache method", () => {
    it("should clear both cache and pending requests", async () => {
      const mockResponse = { data: "test" };
      mockFetch.mockResolvedValue({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResponse),
      });

      // Setup cache
      await apiClient.get("/test");
      expect(mockFetch).toHaveBeenCalledTimes(1);

      // Clear cache
      apiClient.clearCache();

      // Should make new request
      await apiClient.get("/test");
      expect(mockFetch).toHaveBeenCalledTimes(2);
    });
  });
});

describe("feedsApi", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiClient.clearCache();
  });

  describe("getFeeds", () => {
    it("should transform backend feed items correctly", async () => {
      const mockBackendData: BackendFeedItem[] = [
        {
          title: "Test Feed",
          description: "Test Description",
          link: "https://example.com",
          published: "2023-01-01",
        },
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(mockBackendData),
      });

      const result = await feedsApi.getFeeds(1, 10);

      expect(result).toEqual([
        {
          id: "https://example.com",
          title: "Test Feed",
          description: "Test Description",
          link: "https://example.com",
          published: "2023-01-01",
        },
      ]);
    });

    it("should handle empty responses", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(null),
      });

      const result = await feedsApi.getFeeds();

      expect(result).toEqual([]);
    });

    it("should use correct pagination parameters", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue([]),
      });

      await feedsApi.getFeeds(2, 20);

      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost/api/v1/feeds/fetch/limit/40",
        expect.any(Object),
      );
    });
  });

  describe("searchFeeds", () => {
    it("should handle array response from backend", async () => {
      const mockBackendData: BackendFeedItem[] = [
        {
          title: "Search Result",
          description: "Search Description",
          link: "https://search.com",
        },
      ];

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(mockBackendData),
      });

      const result = await feedsApi.searchFeeds("test query");

      expect(result).toEqual({
        results: mockBackendData,
        error: null,
      });
    });

    it("should handle structured response from backend", async () => {
      const mockResponse = {
        results: [
          {
            title: "Search Result",
            description: "Search Description",
            link: "https://search.com",
          },
        ],
        error: null,
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResponse),
      });

      const result = await feedsApi.searchFeeds("test query");

      expect(result).toEqual(mockResponse);
    });

    it("should handle search errors", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      const result = await feedsApi.searchFeeds("test query");

      expect(result).toEqual({
        results: [],
        error: "Network error",
      });
    });

    it("should handle HTTP errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
      });

      const result = await feedsApi.searchFeeds("test query");

      expect(result).toEqual({
        results: [],
        error: "API request failed: 500 Internal Server Error",
      });
    });
  });

  describe("registerRssFeed", () => {
    it("should send POST request with URL", async () => {
      const mockResponse = { message: "Feed registered successfully" };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResponse),
      });

      const result = await feedsApi.registerRssFeed("https://example.com/feed");

      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost/api/v1/rss-feed-link/register",
        expect.objectContaining({
          method: "POST",
          body: JSON.stringify({ url: "https://example.com/feed" }),
        }),
      );
      expect(result).toEqual(mockResponse);
    });
  });

  describe("checkHealth", () => {
    it("should check API health with short cache", async () => {
      const mockResponse = { status: "healthy" };
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResponse),
      });

      const result = await feedsApi.checkHealth();

      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost/api/v1/health",
        expect.any(Object),
      );
      expect(result).toEqual(mockResponse);
    });
  });

  describe("getFeedStats", () => {
    it("should fetch feed statistics with correct endpoint", async () => {
      const mockStatsResponse = {
        feed_amount: { amount: 42 },
        summarized_feed: { amount: 28 }
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(mockStatsResponse),
      });

      const result = await feedsApi.getFeedStats();

      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost/api/v1/feeds/stats",
        expect.objectContaining({
          method: "GET",
          headers: expect.objectContaining({
            "Content-Type": "application/json",
            "Cache-Control": "max-age=300",
            "Accept-Encoding": "gzip, deflate, br",
          }),
          keepalive: true,
        }),
      );
      expect(result).toEqual(mockStatsResponse);
    });

    it("should use 5-minute cache for stats", async () => {
      const mockStatsResponse = {
        feed_amount: { amount: 42 },
        summarized_feed: { amount: 28 }
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(mockStatsResponse),
      });

      // First call
      await feedsApi.getFeedStats();
      expect(mockFetch).toHaveBeenCalledTimes(1);

      // Second call should use cache
      await feedsApi.getFeedStats();
      expect(mockFetch).toHaveBeenCalledTimes(1);
    });

    it("should handle empty stats response", async () => {
      const emptyStatsResponse = {
        feed_amount: { amount: 0 },
        summarized_feed: { amount: 0 }
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(emptyStatsResponse),
      });

      const result = await feedsApi.getFeedStats();
      expect(result).toEqual(emptyStatsResponse);
    });

    it("should handle API errors for stats", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 500,
        statusText: "Internal Server Error",
      });

      await expect(feedsApi.getFeedStats()).rejects.toThrow(
        "API request failed: 500 Internal Server Error"
      );
    });

    it("should handle network errors for stats", async () => {
      mockFetch.mockRejectedValueOnce(new Error("Network error"));

      await expect(feedsApi.getFeedStats()).rejects.toThrow("Network error");
    });
  });

  describe("prefetchFeeds", () => {
    it("should prefetch multiple pages", async () => {
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: vi.fn().mockResolvedValue([]),
        })
        .mockResolvedValueOnce({
          ok: true,
          json: vi.fn().mockResolvedValue([]),
        });

      await feedsApi.prefetchFeeds([0, 1]);

      expect(mockFetch).toHaveBeenCalledTimes(2);
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost/api/v1/feeds/fetch/page/0",
        expect.any(Object),
      );
      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost/api/v1/feeds/fetch/page/1",
        expect.any(Object),
      );
    });

    it("should handle prefetch errors gracefully", async () => {
      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: vi.fn().mockResolvedValue([]),
        })
        .mockRejectedValueOnce(new Error("Network error"));

      // Should not throw error
      await expect(feedsApi.prefetchFeeds([0, 1])).resolves.toBeUndefined();
    });
  });
});

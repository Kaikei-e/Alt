import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";
import { apiClient } from "@/lib/api";

// Mock fetch globally
const mockFetch = vi.fn();
vi.stubGlobal("fetch", mockFetch);

describe("Cursor-based API", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    apiClient.clearCache();
    vi.useFakeTimers();
    mockFetch.mockClear();
  });

  afterEach(() => {
    vi.useRealTimers();
    mockFetch.mockReset();
  });

  describe("getFeedsWithCursor", () => {
    it("should make GET request to cursor endpoint without cursor (first page)", async () => {
      const mockResponse = {
        data: [
          {
            title: "Test Feed 1",
            description: "Description 1",
            link: "https://example.com/1",
            published: "2023-01-01T00:00:00Z",
          },
        ],
        next_cursor: "2023-01-01T00:00:00Z",
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResponse),
      });

      // Mock the future implementation
      const getFeedsWithCursor = async (cursor?: string, limit: number = 20) => {
        const params = new URLSearchParams();
        params.set("limit", limit.toString());
        if (cursor) {
          params.set("cursor", cursor);
        }
        
        return apiClient.get(`/v1/feeds/fetch/cursor?${params.toString()}`, 10);
      };

      const result = await getFeedsWithCursor(undefined, 20);

      expect(mockFetch).toHaveBeenCalledWith(
        "http://localhost/api/v1/feeds/fetch/cursor?limit=20",
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

    it("should make GET request with cursor parameter (subsequent pages)", async () => {
      const mockResponse = {
        data: [
          {
            title: "Test Feed 2",
            description: "Description 2",
            link: "https://example.com/2",
            published: "2022-12-31T23:59:59Z",
          },
        ],
        next_cursor: "2022-12-31T23:59:59Z",
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResponse),
      });

      const getFeedsWithCursor = async (cursor?: string, limit: number = 20) => {
        const params = new URLSearchParams();
        params.set("limit", limit.toString());
        if (cursor) {
          params.set("cursor", cursor);
        }
        
        return apiClient.get(`/v1/feeds/fetch/cursor?${params.toString()}`, 10);
      };

      const testCursor = "2023-01-01T00:00:00Z";
      const result = await getFeedsWithCursor(testCursor, 10);

      expect(mockFetch).toHaveBeenCalledWith(
        `http://localhost/api/v1/feeds/fetch/cursor?limit=10&cursor=${encodeURIComponent(testCursor)}`,
        expect.any(Object),
      );

      expect(result).toEqual(mockResponse);
    });

    it("should validate limit parameter constraints", async () => {
      const getFeedsWithCursor = async (cursor?: string, limit: number = 20) => {
        // Validate limit constraints
        if (limit < 1 || limit > 100) {
          throw new Error("Limit must be between 1 and 100");
        }
        
        const params = new URLSearchParams();
        params.set("limit", limit.toString());
        if (cursor) {
          params.set("cursor", cursor);
        }
        
        return apiClient.get(`/v1/feeds/fetch/cursor?${params.toString()}`, 10);
      };

      // Test limit too small
      await expect(getFeedsWithCursor(undefined, 0)).rejects.toThrow(
        "Limit must be between 1 and 100"
      );

      // Test limit too large
      await expect(getFeedsWithCursor(undefined, 101)).rejects.toThrow(
        "Limit must be between 1 and 100"
      );

      // Test valid limit
      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue({ data: [], next_cursor: null }),
      });

      await expect(getFeedsWithCursor(undefined, 50)).resolves.toBeDefined();
    });

    it("should handle empty response (no more feeds)", async () => {
      const mockResponse = {
        data: [],
        next_cursor: null,
      };

      mockFetch.mockResolvedValueOnce({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResponse),
      });

      const getFeedsWithCursor = async (cursor?: string, limit: number = 20) => {
        const params = new URLSearchParams();
        params.set("limit", limit.toString());
        if (cursor) {
          params.set("cursor", cursor);
        }
        
        return apiClient.get(`/v1/feeds/fetch/cursor?${params.toString()}`, 10);
      };

      const result = await getFeedsWithCursor("2022-01-01T00:00:00Z", 20);

      expect(result.data).toEqual([]);
      expect(result.next_cursor).toBeNull();
    });

    it("should handle invalid cursor format errors", async () => {
      mockFetch.mockResolvedValueOnce({
        ok: false,
        status: 400,
        statusText: "Bad Request",
      });

      const getFeedsWithCursor = async (cursor?: string, limit: number = 20) => {
        const params = new URLSearchParams();
        params.set("limit", limit.toString());
        if (cursor) {
          params.set("cursor", cursor);
        }
        
        return apiClient.get(`/v1/feeds/fetch/cursor?${params.toString()}`, 10);
      };

      const invalidCursor = "invalid-date-format";
      
      await expect(getFeedsWithCursor(invalidCursor, 20)).rejects.toThrow(
        "API request failed: 400 Bad Request"
      );
    });

    it("should use appropriate cache TTL for cursor requests", async () => {
      const mockResponse = {
        data: [
          {
            title: "Cached Feed",
            description: "Cached Description",
            link: "https://cached.com",
            published: "2023-01-01T00:00:00Z",
          },
        ],
        next_cursor: "2023-01-01T00:00:00Z",
      };

      mockFetch.mockResolvedValue({
        ok: true,
        json: vi.fn().mockResolvedValue(mockResponse),
      });

      const getFeedsWithCursor = async (cursor?: string, limit: number = 20) => {
        const params = new URLSearchParams();
        params.set("limit", limit.toString());
        if (cursor) {
          params.set("cursor", cursor);
        }
        
        // Use different cache TTL based on whether it's first page or not
        const cacheTtl = cursor ? 15 : 5; // 15 min for subsequent pages, 5 min for first page
        return apiClient.get(`/v1/feeds/fetch/cursor?${params.toString()}`, cacheTtl);
      };

      // First call (should cache)
      await getFeedsWithCursor(undefined, 20);
      
      // Second call should use cache
      await getFeedsWithCursor(undefined, 20);

      expect(mockFetch).toHaveBeenCalledTimes(1);
    });
  });

  describe("InfiniteScrollCursor", () => {
    it("should maintain cursor state for infinite scroll", async () => {
      // Mock sequence of responses for infinite scroll
      const responses = [
        {
          data: [
            { title: "Feed 1", description: "Desc 1", link: "https://1.com", published: "2023-01-03T00:00:00Z" },
            { title: "Feed 2", description: "Desc 2", link: "https://2.com", published: "2023-01-02T00:00:00Z" },
          ],
          next_cursor: "2023-01-02T00:00:00Z",
        },
        {
          data: [
            { title: "Feed 3", description: "Desc 3", link: "https://3.com", published: "2023-01-01T00:00:00Z" },
          ],
          next_cursor: "2023-01-01T00:00:00Z",
        },
        {
          data: [],
          next_cursor: null,
        },
      ];

      mockFetch
        .mockResolvedValueOnce({
          ok: true,
          json: vi.fn().mockResolvedValue(responses[0]),
        })
        .mockResolvedValueOnce({
          ok: true,
          json: vi.fn().mockResolvedValue(responses[1]),
        })
        .mockResolvedValueOnce({
          ok: true,
          json: vi.fn().mockResolvedValue(responses[2]),
        });

      // Mock cursor state management
      class CursorPagination {
        private cursor: string | null = null;
        private hasMore = true;

        async loadNextPage(limit: number = 20) {
          if (!this.hasMore) {
            return { data: [], hasMore: false };
          }

          const params = new URLSearchParams();
          params.set("limit", limit.toString());
          if (this.cursor) {
            params.set("cursor", this.cursor);
          }

          const response = await apiClient.get(`/v1/feeds/fetch/cursor?${params.toString()}`, 10);
          
          this.cursor = response.next_cursor;
          this.hasMore = response.next_cursor !== null;

          return {
            data: response.data,
            hasMore: this.hasMore,
          };
        }
      }

      const pagination = new CursorPagination();

      // Load first page
      const page1 = await pagination.loadNextPage(2);
      expect(page1.data).toHaveLength(2);
      expect(page1.hasMore).toBe(true);

      // Load second page
      const page2 = await pagination.loadNextPage(2);
      expect(page2.data).toHaveLength(1);
      expect(page2.hasMore).toBe(true);

      // Load third page (empty)
      const page3 = await pagination.loadNextPage(2);
      expect(page3.data).toHaveLength(0);
      expect(page3.hasMore).toBe(false);

      expect(mockFetch).toHaveBeenCalledTimes(3);
    });
  });
});
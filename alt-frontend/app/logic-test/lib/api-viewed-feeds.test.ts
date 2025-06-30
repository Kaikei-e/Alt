import { describe, it, expect, vi, beforeEach } from "vitest";
import { feedsApi, apiClient } from "@/lib/api";
import { BackendFeedItem } from "@/schema/feed";
import { CursorResponse } from "@/lib/api";

describe("ReadFeeds API Client - TDD Implementation", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("getReadFeedsWithCursor", () => {
    const mockBackendFeedItems: BackendFeedItem[] = [
      {
        title: "Test Feed 1",
        description: "Test Description 1",
        link: "https://example.com/feed1",
        published: "2024-01-01T00:00:00Z",
      },
      {
        title: "Test Feed 2",
        description: "Test Description 2",
        link: "https://example.com/feed2",
        published: "2024-01-02T00:00:00Z",
      },
    ];

    const mockCursorResponse: CursorResponse<BackendFeedItem> = {
      data: mockBackendFeedItems,
      next_cursor: "cursor123",
    };

    it("should fetch read feeds with default parameters", async () => {
      // Spy on apiClient.get method
      const mockGet = vi.spyOn(apiClient, "get");
      mockGet.mockResolvedValue(mockCursorResponse);

      const result = await feedsApi.getReadFeedsWithCursor();

      expect(mockGet).toHaveBeenCalledWith(
        "/v1/feeds/fetch/viewed/cursor?limit=20",
        10,
      );
      expect(result).toEqual({
        data: [
          {
            id: "https://example.com/feed1",
            title: "Test Feed 1",
            description: "Test Description 1",
            link: "https://example.com/feed1",
            published: "2024-01-01T00:00:00Z",
          },
          {
            id: "https://example.com/feed2",
            title: "Test Feed 2",
            description: "Test Description 2",
            link: "https://example.com/feed2",
            published: "2024-01-02T00:00:00Z",
          },
        ],
        next_cursor: "cursor123",
      });
    });

    it("should fetch read feeds with cursor and custom limit", async () => {
      const mockGet = vi.spyOn(apiClient, "get");
      mockGet.mockResolvedValue(mockCursorResponse);

      const result = await feedsApi.getReadFeedsWithCursor("cursor123", 50);

      expect(mockGet).toHaveBeenCalledWith(
        "/v1/feeds/fetch/viewed/cursor?limit=50&cursor=cursor123",
        15,
      );
      expect(result.data).toHaveLength(2);
      expect(result.next_cursor).toBe("cursor123");
    });

    it("should validate limit constraints (minimum)", async () => {
      await expect(
        feedsApi.getReadFeedsWithCursor(undefined, 0),
      ).rejects.toThrow("Limit must be between 1 and 100");
    });

    it("should validate limit constraints (maximum)", async () => {
      await expect(
        feedsApi.getReadFeedsWithCursor(undefined, 101),
      ).rejects.toThrow("Limit must be between 1 and 100");
    });

    it("should handle API error responses", async () => {
      const mockGet = vi.spyOn(apiClient, "get");
      mockGet.mockRejectedValue(new Error("API Error"));

      await expect(feedsApi.getReadFeedsWithCursor()).rejects.toThrow(
        "API Error",
      );
    });

    it("should transform BackendFeedItem to Feed correctly", async () => {
      const mockBackendItem: BackendFeedItem = {
        title: "Test Title",
        description: "Test Description",
        link: "https://example.com/test",
        published: "2024-01-01T00:00:00Z",
      };

      const mockResponse: CursorResponse<BackendFeedItem> = {
        data: [mockBackendItem],
        next_cursor: null,
      };

      const mockGet = vi.spyOn(apiClient, "get");
      mockGet.mockResolvedValue(mockResponse);

      const result = await feedsApi.getReadFeedsWithCursor();

      expect(result.data[0]).toEqual({
        id: "https://example.com/test",
        title: "Test Title",
        description: "Test Description",
        link: "https://example.com/test",
        published: "2024-01-01T00:00:00Z",
      });
    });
  });

  describe("prefetchReadFeeds", () => {
    it("should prefetch read feeds with given cursors", async () => {
      const cursors = ["cursor1", "cursor2", "cursor3"];
      const mockGet = vi.spyOn(apiClient, "get");
      mockGet.mockResolvedValue({ data: [], next_cursor: null });

      await feedsApi.prefetchReadFeeds(cursors);

      expect(mockGet).toHaveBeenCalledTimes(3);
      expect(mockGet).toHaveBeenCalledWith(
        "/v1/feeds/fetch/viewed/cursor?limit=20&cursor=cursor1",
        15,
      );
      expect(mockGet).toHaveBeenCalledWith(
        "/v1/feeds/fetch/viewed/cursor?limit=20&cursor=cursor2",
        15,
      );
      expect(mockGet).toHaveBeenCalledWith(
        "/v1/feeds/fetch/viewed/cursor?limit=20&cursor=cursor3",
        15,
      );
    });

    it("should handle prefetch errors gracefully", async () => {
      const cursors = ["cursor1", "cursor2"];
      const mockGet = vi.spyOn(apiClient, "get");
      mockGet
        .mockResolvedValueOnce({ data: [], next_cursor: null })
        .mockRejectedValueOnce(new Error("Prefetch error"));

      // Should not throw error even if one prefetch fails
      await expect(
        feedsApi.prefetchReadFeeds(cursors),
      ).resolves.toBeUndefined();
    });
  });
});

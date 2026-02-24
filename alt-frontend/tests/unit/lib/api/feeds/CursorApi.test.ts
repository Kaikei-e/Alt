import { beforeEach, describe, expect, it, vi } from "vitest";
import { CursorApi } from "../../../../../src/lib/api/feeds/CursorApi";

// Mock ApiClient
vi.mock("../core/ApiClient");

describe("CursorApi", () => {
  let mockApiClient: any;
  let cursorApi: CursorApi<any, any>;

  beforeEach(() => {
    mockApiClient = {
      get: vi.fn(),
    } as any;

    const transformer = (item: { id: number; name: string }) => ({
      id: item.id,
      displayName: item.name.toUpperCase(),
    });

    cursorApi = new CursorApi(
      mockApiClient,
      "/api/items",
      transformer,
      10, // default cache TTL
    );
  });

  describe("fetchWithCursor", () => {
    it("should fetch data without cursor", async () => {
      const mockResponse = {
        data: [
          { id: 1, name: "item1" },
          { id: 2, name: "item2" },
        ],
        next_cursor: "cursor123",
      };

      mockApiClient.get.mockResolvedValueOnce(mockResponse);

      const result = await cursorApi.fetchWithCursor();

      expect(mockApiClient.get).toHaveBeenCalledWith("/api/items?limit=20", 10);

      expect(result).toEqual({
        data: [
          { id: 1, displayName: "ITEM1" },
          { id: 2, displayName: "ITEM2" },
        ],
        next_cursor: "cursor123",
        has_more: true,
      });
    });

    it("should fetch data with cursor", async () => {
      const mockResponse = {
        data: [{ id: 3, name: "item3" }],
        next_cursor: null,
      };

      mockApiClient.get.mockResolvedValueOnce(mockResponse);

      const result = await cursorApi.fetchWithCursor("cursor123", 10);

      expect(mockApiClient.get).toHaveBeenCalledWith(
        "/api/items?limit=10&cursor=cursor123",
        15, // increased cache TTL for cursored requests
      );

      expect(result).toEqual({
        data: [{ id: 3, displayName: "ITEM3" }],
        next_cursor: null,
        has_more: false,
      });
    });

    it("should validate limit constraints", async () => {
      await expect(cursorApi.fetchWithCursor(undefined, 0)).rejects.toThrow(
        "Limit must be between 1 and 100",
      );

      await expect(cursorApi.fetchWithCursor(undefined, 101)).rejects.toThrow(
        "Limit must be between 1 and 100",
      );
    });

    it("should handle malformed response gracefully", async () => {
      mockApiClient.get.mockResolvedValueOnce(null);

      const result = await cursorApi.fetchWithCursor();

      expect(result).toEqual({
        data: [],
        next_cursor: null,
        has_more: false,
      });
    });

    it("should handle response with non-array data", async () => {
      mockApiClient.get.mockResolvedValueOnce({
        data: "invalid",
        next_cursor: null,
      });

      const result = await cursorApi.fetchWithCursor();

      expect(result).toEqual({
        data: [],
        next_cursor: null,
        has_more: false,
      });
    });

    it("should transform all items correctly", async () => {
      const mockResponse = {
        data: [
          { id: 1, name: "apple" },
          { id: 2, name: "banana" },
          { id: 3, name: "cherry" },
        ],
        next_cursor: "next",
      };

      mockApiClient.get.mockResolvedValueOnce(mockResponse);

      const result = await cursorApi.fetchWithCursor();

      expect(result.data).toEqual([
        { id: 1, displayName: "APPLE" },
        { id: 2, displayName: "BANANA" },
        { id: 3, displayName: "CHERRY" },
      ]);
    });

    it("should use different cache TTL for cursor vs non-cursor requests", async () => {
      const mockResponse = { data: [], next_cursor: null };
      mockApiClient.get.mockResolvedValue(mockResponse);

      // Non-cursor request
      await cursorApi.fetchWithCursor();
      expect(mockApiClient.get).toHaveBeenCalledWith(
        expect.any(String),
        10, // default TTL
      );

      // Cursor request
      await cursorApi.fetchWithCursor("cursor123");
      expect(mockApiClient.get).toHaveBeenCalledWith(
        expect.any(String),
        15, // increased TTL
      );
    });
  });
});

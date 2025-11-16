import { describe, expect, it } from "vitest";
import {
  ApiClientError,
  apiClient,
  feedApi,
  desktopApi,
  serverFetch,
} from "@/lib/api";

describe("API Integration", () => {
  it("should export all required API components", () => {
    expect(feedApi).toBeDefined();
    expect(apiClient).toBeDefined();
    expect(ApiClientError).toBeDefined();
    expect(serverFetch).toBeDefined();
  });

  it("should have all main feedApi methods", () => {
    expect(typeof feedApi.checkHealth).toBe("function");
    expect(typeof feedApi.getFeeds).toBe("function");
    expect(typeof feedApi.getFeedsWithCursor).toBe("function");
    expect(typeof feedApi.registerRssFeed).toBe("function");
    expect(typeof feedApi.searchFeeds).toBe("function");
    expect(typeof feedApi.getFeedStats).toBe("function");
    expect(typeof feedApi.clearCache).toBe("function");
  });

  it("should have desktop-specific methods", () => {
    expect(typeof desktopApi.getDesktopFeeds).toBe("function");
    expect(typeof desktopApi.getTestFeeds).toBe("function");
    expect(typeof desktopApi.toggleFavorite).toBe("function");
    expect(typeof desktopApi.getRecentActivity).toBe("function");
    expect(typeof desktopApi.getWeeklyStats).toBe("function");
  });

  it("should maintain ApiClientError constructor compatibility", () => {
    const error = new ApiClientError("Test error", 404, "NOT_FOUND");
    expect(error.message).toBe("Test error");
    expect(error.status).toBe(404);
    expect(error.code).toBe("NOT_FOUND");
    expect(error.name).toBe("ApiError");
  });

  it("should have apiClient methods", () => {
    expect(typeof apiClient.get).toBe("function");
    expect(typeof apiClient.post).toBe("function");
    expect(typeof apiClient.clearCache).toBe("function");
    expect(typeof apiClient.destroy).toBe("function");
  });
});

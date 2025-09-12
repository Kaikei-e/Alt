import { describe, it, expect } from "vitest";
import {
  feedsApi,
  apiClient,
  ApiClientError,
  serverFetch,
} from "../../../src/api";

describe("API Integration", () => {
  it("should export all required API components", () => {
    expect(feedsApi).toBeDefined();
    expect(apiClient).toBeDefined();
    expect(ApiClientError).toBeDefined();
    expect(serverFetch).toBeDefined();
  });

  it("should have all main feedsApi methods", () => {
    expect(typeof feedsApi.checkHealth).toBe("function");
    expect(typeof feedsApi.getFeeds).toBe("function");
    expect(typeof feedsApi.getFeedsWithCursor).toBe("function");
    expect(typeof feedsApi.registerRssFeed).toBe("function");
    expect(typeof feedsApi.searchFeeds).toBe("function");
    expect(typeof feedsApi.getFeedStats).toBe("function");
    expect(typeof feedsApi.clearCache).toBe("function");
  });

  it("should have desktop-specific methods", () => {
    expect(typeof feedsApi.getDesktopFeeds).toBe("function");
    expect(typeof feedsApi.getTestFeeds).toBe("function");
    expect(typeof feedsApi.toggleFavorite).toBe("function");
    expect(typeof feedsApi.getRecentActivity).toBe("function");
    expect(typeof feedsApi.getWeeklyStats).toBe("function");
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

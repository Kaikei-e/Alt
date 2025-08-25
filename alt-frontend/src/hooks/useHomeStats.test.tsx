import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useHomeStats } from "./useHomeStats";
import { feedsApi } from "@/lib/api";

// Mock the auth context directly with a simple authenticated user
vi.mock("@/contexts/auth-context", () => ({
  useAuth: vi.fn(() => ({
    isAuthenticated: true,
    user: { id: "test-user", email: "test@example.com" },
    loading: false,
    error: null,
    login: vi.fn(),
    logout: vi.fn(),
    checkSession: vi.fn(),
  })),
}));

// Mock the API
vi.mock("@/lib/api", () => ({
  feedsApi: {
    getFeedStats: vi.fn(),
    getTodayUnreadCount: vi.fn(),
  },
}));

// Mock the useTodayUnreadCount hook
vi.mock("./useTodayUnreadCount", () => ({
  useTodayUnreadCount: vi.fn(() => ({
    count: 0,
    isLoading: false,
  })),
}));

describe("useHomeStats", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should return initial loading state", () => {
    const { result } = renderHook(() => useHomeStats());

    expect(result.current.feedStats).toBeNull();
    expect(result.current.isLoadingStats).toBe(true);
    expect(result.current.statsError).toBeNull();
    expect(result.current.unreadCount).toBe(0);
  });

  it("should fetch and return feed stats successfully", async () => {
    const mockFeedStats = {
      feed_amount: { amount: 24 },
      summarized_feed: { amount: 18 },
    };

    const mockUseTodayUnreadCount = await import("./useTodayUnreadCount");
    vi.mocked(mockUseTodayUnreadCount.useTodayUnreadCount).mockReturnValue({
      count: 156,
      isLoading: false,
    });

    vi.mocked(feedsApi.getFeedStats).mockResolvedValue(mockFeedStats);

    const { result } = renderHook(() => useHomeStats());

    await waitFor(() => {
      expect(result.current.isLoadingStats).toBe(false);
    });

    expect(result.current.feedStats).toEqual(mockFeedStats);
    expect(result.current.unreadCount).toBe(156);
    expect(result.current.statsError).toBeNull();
  });

  it("should handle API error gracefully", async () => {
    const mockUseTodayUnreadCount = await import("./useTodayUnreadCount");
    vi.mocked(mockUseTodayUnreadCount.useTodayUnreadCount).mockReturnValue({
      count: 0,
      isLoading: false,
    });

    vi.mocked(feedsApi.getFeedStats).mockRejectedValue(new Error("API Error"));

    const { result } = renderHook(() => useHomeStats());

    await waitFor(() => {
      expect(result.current.isLoadingStats).toBe(false);
    });

    expect(result.current.feedStats).toBeNull();
    expect(result.current.statsError).toBe("Failed to fetch feed stats");
  });

  it("should provide extraStats with calculated values", async () => {
    const mockFeedStats = {
      feed_amount: { amount: 24 },
      summarized_feed: { amount: 18 },
    };

    const mockUseTodayUnreadCount = await import("./useTodayUnreadCount");
    vi.mocked(mockUseTodayUnreadCount.useTodayUnreadCount).mockReturnValue({
      count: 156,
      isLoading: false,
    });

    vi.mocked(feedsApi.getFeedStats).mockResolvedValue(mockFeedStats);

    const { result } = renderHook(() => useHomeStats());

    await waitFor(() => {
      expect(result.current.isLoadingStats).toBe(false);
    });

    expect(result.current.extraStats).toEqual({
      weeklyReads: 45,
      aiProcessed: 18,
      bookmarks: 12,
    });
  });

  it("should provide refreshStats function", async () => {
    const mockFeedStats = {
      feed_amount: { amount: 24 },
      summarized_feed: { amount: 18 },
    };

    const mockUseTodayUnreadCount = await import("./useTodayUnreadCount");
    vi.mocked(mockUseTodayUnreadCount.useTodayUnreadCount).mockReturnValue({
      count: 156,
      isLoading: false,
    });

    vi.mocked(feedsApi.getFeedStats).mockResolvedValue(mockFeedStats);

    const { result } = renderHook(() => useHomeStats());

    await waitFor(() => {
      expect(result.current.isLoadingStats).toBe(false);
    });

    expect(typeof result.current.refreshStats).toBe("function");

    // Call refreshStats and verify it triggers another API call
    await result.current.refreshStats();
    expect(feedsApi.getFeedStats).toHaveBeenCalledTimes(2);
  });
});
import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useRecentActivity } from "./useRecentActivity";
import { feedsApi } from "@/lib/api";
import { AuthProvider } from "@/contexts/auth-context";
import { ReactNode } from "react";

// Mock the API
vi.mock("@/lib/api", () => ({
  feedsApi: {
    getRecentActivity: vi.fn(),
  },
}));

// Mock AuthProvider wrapper
const AuthProviderWrapper = ({ children }: { children: ReactNode }) => {
  return <AuthProvider>{children}</AuthProvider>;
};

describe("useRecentActivity", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should return initial loading state", () => {
    const { result } = renderHook(() => useRecentActivity(), {
      wrapper: AuthProviderWrapper,
    });

    expect(result.current.activities).toEqual([]);
    expect(result.current.isLoading).toBe(true);
    expect(result.current.error).toBeNull();
  });

  it("should fetch and return activity data successfully", async () => {
    const mockActivities = [
      {
        id: 1,
        type: "new_feed" as const,
        title: "Added new RSS feed",
        timestamp: new Date().toISOString(),
      },
      {
        id: 2,
        type: "ai_summary" as const,
        title: "AI summary generated",
        timestamp: new Date(Date.now() - 3600000).toISOString(), // 1 hour ago
      },
    ];

    vi.mocked(feedsApi.getRecentActivity).mockResolvedValue(mockActivities);

    const { result } = renderHook(() => useRecentActivity(), {
      wrapper: AuthProviderWrapper,
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.activities).toHaveLength(2);
    expect(result.current.activities[0]).toEqual({
      id: 1,
      type: "new_feed",
      title: "Added new RSS feed",
      time: expect.any(String),
    });
    expect(result.current.error).toBeNull();
  });

  it("should handle API error gracefully", async () => {
    vi.mocked(feedsApi.getRecentActivity).mockRejectedValue(
      new Error("API Error"),
    );

    const { result } = renderHook(() => useRecentActivity(), {
      wrapper: AuthProviderWrapper,
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.activities).toEqual([]);
    expect(result.current.error).toBe("Failed to fetch recent activity");
  });

  it("should transform timestamps to relative time", async () => {
    const mockActivities = [
      {
        id: 1,
        type: "new_feed" as const,
        title: "Test activity",
        timestamp: new Date(Date.now() - 7200000).toISOString(), // 2 hours ago
      },
    ];

    vi.mocked(feedsApi.getRecentActivity).mockResolvedValue(mockActivities);

    const { result } = renderHook(() => useRecentActivity(), {
      wrapper: AuthProviderWrapper,
    });

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });

    expect(result.current.activities[0].time).toContain("hours ago");
  });

  it("should accept custom limit parameter", async () => {
    const mockActivities = [
      {
        id: 1,
        type: "new_feed" as const,
        title: "Test activity",
        timestamp: new Date().toISOString(),
      },
    ];

    vi.mocked(feedsApi.getRecentActivity).mockResolvedValue(mockActivities);

    renderHook(() => useRecentActivity(5), {
      wrapper: AuthProviderWrapper,
    });

    await waitFor(() => {
      expect(feedsApi.getRecentActivity).toHaveBeenCalledWith(5);
    });
  });
});

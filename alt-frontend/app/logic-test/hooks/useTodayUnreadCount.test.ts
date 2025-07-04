import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useTodayUnreadCount } from "@/hooks/useTodayUnreadCount";
import { feedsApi } from "@/lib/api";

vi.mock("@/lib/api", () => ({
  feedsApi: {
    getTodayUnreadCount: vi.fn(),
  },
}));

vi.mock("@/lib/utils/time", () => ({
  getStartOfLocalDayUTC: () => new Date("2024-05-26T15:00:00.000Z"),
}));

describe("useTodayUnreadCount", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should fetch unread count on mount", async () => {
    vi.mocked(feedsApi.getTodayUnreadCount).mockResolvedValue({ count: 7 });
    const { result } = renderHook(() => useTodayUnreadCount());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
    expect(result.current.count).toBe(7);
  });

  it("should handle errors", async () => {
    vi.mocked(feedsApi.getTodayUnreadCount).mockRejectedValue(new Error("err"));
    const { result } = renderHook(() => useTodayUnreadCount());

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
    });
    expect(result.current.count).toBe(0);
  });
});

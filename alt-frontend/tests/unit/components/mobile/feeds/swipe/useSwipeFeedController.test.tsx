import { renderHook, waitFor } from "@testing-library/react";
import { vi, describe, expect, beforeEach, afterEach, it } from "vitest";
import type { CursorResponse } from "@/schema/common";
import type { Feed } from "@/schema/feed";
import { useSwipeFeedController } from "@/components/mobile/feeds/swipe/useSwipeFeedController";

const mockUseSWRInfinite = vi.fn();

vi.mock("swr/infinite", () => ({
  __esModule: true,
  default: (...args: unknown[]) => mockUseSWRInfinite(...args),
}));

const mockPrefetch = {
  triggerPrefetch: vi.fn(),
  getCachedContent: vi.fn(),
  markAsDismissed: vi.fn(),
};

vi.mock("@/hooks/useArticleContentPrefetch", () => ({
  useArticleContentPrefetch: () => mockPrefetch,
}));

const mockFeedApi = vi.hoisted(() => ({
  getReadFeedsWithCursor: vi.fn(),
  getFeedsWithCursor: vi.fn(),
  updateFeedReadStatus: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  feedApi: mockFeedApi,
}));

const baseFeed: Feed = {
  id: "",
  title: "",
  description: "",
  link: "",
  published: "2024-01-01T00:00:00.000Z",
};

describe("useSwipeFeedController", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockFeedApi.getReadFeedsWithCursor.mockReset();
    mockFeedApi.getFeedsWithCursor.mockReset();
    mockFeedApi.updateFeedReadStatus.mockReset();
    mockUseSWRInfinite.mockImplementation(() => ({
      data: [
        {
          data: [
            {
              ...baseFeed,
              id: "read-feed",
              title: "既読フィード",
              link: "https://example.com/article-1?utm_source=rss",
            },
            {
              ...baseFeed,
              id: "unread-feed",
              title: "未読フィード",
              link: "https://example.com/article-2",
            },
          ],
          next_cursor: null,
        },
      ],
      error: null,
      isLoading: false,
      isValidating: false,
      setSize: vi.fn(),
      mutate: vi.fn(),
    }));

    mockFeedApi.getReadFeedsWithCursor.mockResolvedValue({
      data: [
        {
          ...baseFeed,
          id: "read-feed",
          link: "https://example.com/article-1",
        },
      ],
      next_cursor: null,
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("excludes feeds that were already marked as read via readFeeds initialization", async () => {
    const { result } = renderHook(() => useSwipeFeedController());

    await waitFor(() => {
      expect(result.current.feeds).toHaveLength(1);
    });

    expect(result.current.feeds[0].id).toBe("unread-feed");
    expect(mockFeedApi.getReadFeedsWithCursor).toHaveBeenCalledWith(
      undefined,
      32, // Changed from 100 to 32 for performance optimization
    );
  });

  it("prefetches the next page when feeds are empty but has_more is true and cursor must be derived", async () => {
    const setSizeMock = vi.fn();
    mockUseSWRInfinite.mockImplementation(() => ({
      data: [
        {
          data: [
            {
              ...baseFeed,
              id: "derived-feed",
              link: "https://example.com/article-1",
              published: "2024-01-02T00:00:00.000Z",
            },
          ],
          next_cursor: null,
          has_more: true,
        },
      ],
      error: null,
      isLoading: false,
      isValidating: false,
      setSize: setSizeMock,
      mutate: vi.fn(),
    }));

    mockFeedApi.getReadFeedsWithCursor.mockResolvedValue({
      data: [
        {
          ...baseFeed,
          id: "derived-feed",
          link: "https://example.com/article-1",
          published: "2024-01-01T00:00:00.000Z",
        },
      ],
      next_cursor: null,
    });

    renderHook(() => useSwipeFeedController());

    await waitFor(() => {
      expect(setSizeMock).toHaveBeenCalled();
    });

    expect(typeof setSizeMock.mock.calls[0][0]).toBe("function");
  });

  it("does not prefetch when SWR is validating to avoid infinite loops", async () => {
    const setSizeMock = vi.fn();
    mockUseSWRInfinite.mockImplementation(() => ({
      data: [
        {
          data: [],
          next_cursor: "next-cursor",
          has_more: true,
        },
      ],
      error: null,
      isLoading: false,
      isValidating: true, // Validating prevents prefetch to avoid loops
      setSize: setSizeMock,
      mutate: vi.fn(),
    }));

    mockFeedApi.getReadFeedsWithCursor.mockResolvedValue({
      data: [],
      next_cursor: null,
    });

    renderHook(() => useSwipeFeedController());

    // Wait a bit to ensure prefetch doesn't fire
    await new Promise((resolve) => setTimeout(resolve, 100));

    // setSize should NOT be called when isValidating is true
    expect(setSizeMock).not.toHaveBeenCalled();
  });

  it("prefetches next page when feeds are exhausted and hasMore is true", async () => {
    const nextCursor = "cursor-fallback";
    const feeds = Array.from({ length: 20 }).map((_, index) => ({
      ...baseFeed,
      id: `feed-${index}`,
      link: `https://example.com/article-${index}`,
      published: `2025-01-01T00:00:${String(index).padStart(2, "0")}Z`,
    }));

    const setSizeMock = vi.fn((updater) => {
      return typeof updater === "function" ? updater(1) : updater;
    });

    type TestSwrKey = readonly ["mobile-feed-swipe", string | undefined, number];

    let capturedGetKey:
      | ((
        pageIndex: number,
        previousPageData: CursorResponse<Feed> | null,
      ) => TestSwrKey | null)
      | null = null;
    let capturedFetcher:
      | ((...args: unknown[]) => Promise<CursorResponse<Feed>>)
      | null = null;

    mockFeedApi.getReadFeedsWithCursor.mockResolvedValue({
      data: feeds,
      next_cursor: null,
    });

    mockFeedApi.getFeedsWithCursor.mockResolvedValue({
      data: [],
      next_cursor: null,
    });

    mockUseSWRInfinite.mockImplementation((keyFn, fetcher) => {
      capturedGetKey = keyFn as typeof capturedGetKey;
      capturedFetcher = fetcher as typeof capturedFetcher;

      return {
        data: [
          {
            data: feeds,
            next_cursor: nextCursor,
            has_more: true,
          },
        ],
        error: null,
        isLoading: false,
        isValidating: false, // Changed to false to allow prefetch
        setSize: setSizeMock,
        mutate: vi.fn(),
      };
    });

    renderHook(() => useSwipeFeedController());

    await waitFor(() => {
      expect(setSizeMock).toHaveBeenCalled();
    });
  });

  it("retries prefetch when feeds are empty, hasMore is true, and isValidating becomes false", async () => {
    const nextCursor = "2025-11-20T11:41:37Z";
    const setSizeMock = vi.fn((updater) => {
      return typeof updater === "function" ? updater(1) : updater;
    });

    // All feeds in the page are marked as read, so filtered feeds will be empty
    mockFeedApi.getReadFeedsWithCursor.mockResolvedValue({
      data: [
        {
          ...baseFeed,
          id: "read-feed-1",
          link: "https://example.com/article-1",
        },
        {
          ...baseFeed,
          id: "read-feed-2",
          link: "https://example.com/article-2",
        },
      ],
      next_cursor: null,
    });

    // First render: isValidating is true, so prefetch should not fire
    mockUseSWRInfinite.mockImplementation(() => {
      return {
        data: [
          {
            data: [
              {
                ...baseFeed,
                id: "read-feed-1",
                link: "https://example.com/article-1",
              },
              {
                ...baseFeed,
                id: "read-feed-2",
                link: "https://example.com/article-2",
              },
            ],
            next_cursor: nextCursor,
            has_more: true,
          },
        ],
        error: null,
        isLoading: false,
        isValidating: true, // Initially validating
        setSize: setSizeMock,
        mutate: vi.fn(),
      };
    });

    const { rerender } = renderHook(() => useSwipeFeedController());

    // Wait a bit to ensure prefetch doesn't fire while validating
    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(setSizeMock).not.toHaveBeenCalled();

    // Second render: isValidating becomes false, which should trigger prefetch
    mockUseSWRInfinite.mockImplementation(() => {
      return {
        data: [
          {
            data: [
              {
                ...baseFeed,
                id: "read-feed-1",
                link: "https://example.com/article-1",
              },
              {
                ...baseFeed,
                id: "read-feed-2",
                link: "https://example.com/article-2",
              },
            ],
            next_cursor: nextCursor,
            has_more: true,
          },
        ],
        error: null,
        isLoading: false,
        isValidating: false, // Validation completed
        setSize: setSizeMock,
        mutate: vi.fn(),
      };
    });

    rerender();

    await waitFor(
      () => {
        expect(setSizeMock).toHaveBeenCalled();
      },
      { timeout: 2000 }
    );

    // Verify setSize was called with a function
    expect(typeof setSizeMock.mock.calls[0][0]).toBe("function");
  });

  it("marks feed supply as exhausted after repeated empty prefetch attempts", async () => {
    const nextCursor = "cursor-empty-loop";
    const setSizeMock = vi.fn();

    mockFeedApi.getReadFeedsWithCursor.mockResolvedValue({
      data: [
        {
          ...baseFeed,
          id: "read-feed-1",
          link: "https://example.com/article-1",
        },
        {
          ...baseFeed,
          id: "read-feed-2",
          link: "https://example.com/article-2",
        },
      ],
      next_cursor: null,
    });

    mockUseSWRInfinite.mockImplementation(() => {
      return {
        data: [
          {
            data: [
              {
                ...baseFeed,
                id: "read-feed-1",
                link: "https://example.com/article-1",
              },
              {
                ...baseFeed,
                id: "read-feed-2",
                link: "https://example.com/article-2",
              },
            ],
            next_cursor: nextCursor,
            has_more: true,
          },
        ],
        error: null,
        isLoading: false,
        isValidating: false,
        setSize: setSizeMock,
        mutate: vi.fn(),
      };
    });

    const { result, rerender } = renderHook(() => useSwipeFeedController());

    await waitFor(() => {
      expect(setSizeMock).toHaveBeenCalledTimes(1);
    });

    rerender();
    await waitFor(() => {
      expect(setSizeMock).toHaveBeenCalledTimes(2);
    });

    rerender();
    await waitFor(() => {
      expect(result.current.hasMore).toBe(false);
    });

    rerender();
    await new Promise((resolve) => setTimeout(resolve, 50));
    expect(setSizeMock).toHaveBeenCalledTimes(3);
  });
});


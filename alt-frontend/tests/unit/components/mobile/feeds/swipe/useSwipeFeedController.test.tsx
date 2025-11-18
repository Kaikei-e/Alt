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
      100,
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

  it("prefetches even when SWR is validating but feeds become empty", async () => {
    const setSizeMock = vi.fn();
    mockUseSWRInfinite.mockImplementation(() => ({
      data: [
        {
          data: [
            {
              ...baseFeed,
              id: "validating-feed",
              link: "https://example.com/article-1",
            },
          ],
          next_cursor: null,
          has_more: true,
        },
      ],
      error: null,
      isLoading: false,
      isValidating: true,
      setSize: setSizeMock,
      mutate: vi.fn(),
    }));

    mockFeedApi.getReadFeedsWithCursor.mockResolvedValue({
      data: [
        {
          ...baseFeed,
          id: "validating-feed",
          link: "https://example.com/article-1",
        },
      ],
      next_cursor: null,
    });

    renderHook(() => useSwipeFeedController());

    await waitFor(() => {
      expect(setSizeMock).toHaveBeenCalled();
    });
  });

  it("uses fallback cursor after reading 20 feeds even while validating", async () => {
    const nextCursor = "cursor-fallback";
    const feeds = Array.from({ length: 20 }).map((_, index) => ({
      ...baseFeed,
      id: `feed-${index}`,
      link: `https://example.com/article-${index}`,
      published: `2025-01-01T00:00:${String(index).padStart(2, "0")}Z`,
    }));

    const setSizeMock = vi.fn((updater) => {
      const keyFn = setSizeMock.getMockImplementation();
      return undefined;
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
        isValidating: true,
        setSize: (updater: (size: number) => number) => {
          const key = capturedGetKey?.(1, null);
          if (key && capturedFetcher) {
            capturedFetcher(...key);
          }
          return typeof updater === "function" ? updater(1) : updater;
        },
        mutate: vi.fn(),
      };
    });

    renderHook(() => useSwipeFeedController());

    await waitFor(() => {
      expect(mockFeedApi.getFeedsWithCursor).toHaveBeenCalledWith(
        nextCursor,
        20,
      );
    });
  });
});


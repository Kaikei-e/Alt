import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, type Mock, vi } from "vitest";

import { useSwipeFeedController } from "@/components/mobile/feeds/swipe/useSwipeFeedController";

const {
  mockUseSWRInfinite,
  mockUpdateFeedReadStatus,
  mockTriggerPrefetch,
  mockGetCachedContent,
  mockMarkAsDismissed,
} = vi.hoisted(() => ({
  mockUseSWRInfinite: vi.fn(),
  mockUpdateFeedReadStatus: vi.fn(),
  mockTriggerPrefetch: vi.fn(),
  mockGetCachedContent: vi.fn(),
  mockMarkAsDismissed: vi.fn(),
}));

vi.mock("swr/infinite", () => ({
  default: (...args: unknown[]) => mockUseSWRInfinite(...args),
  useSWRInfinite: (...args: unknown[]) => mockUseSWRInfinite(...args),
}));

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = (await importOriginal()) as typeof import("@/lib/api");
  return {
    ...actual,
    feedsApi: {
      ...actual.feedsApi,
      getFeedsWithCursor: vi.fn(),
      updateFeedReadStatus: mockUpdateFeedReadStatus,
    },
  };
});

vi.mock("@/hooks/useArticleContentPrefetch", () => ({
  useArticleContentPrefetch: () => ({
    triggerPrefetch: mockTriggerPrefetch,
    getCachedContent: mockGetCachedContent,
    markAsDismissed: mockMarkAsDismissed,
  }),
}));

describe("useSwipeFeedController", () => {
  const feedA = {
    id: "feed-1",
    title: "Feed 1",
    description: "First feed",
    link: "https://example.com/feed-1",
    published: "2025-01-01T00:00:00Z",
  };
  const feedB = {
    id: "feed-2",
    title: "Feed 2",
    description: "Second feed",
    link: "https://example.com/feed-2",
    published: "2025-01-02T00:00:00Z",
  };
  const feedC = {
    id: "feed-3",
    title: "Feed 3",
    description: "Third feed",
    link: "https://example.com/feed-3",
    published: "2025-01-03T00:00:00Z",
  };
  const feedD = {
    id: "feed-4",
    title: "Feed 4",
    description: "Fourth feed",
    link: "https://example.com/feed-4",
    published: "2025-01-04T00:00:00Z",
  };

  let swrState: {
    data: Array<{ data: Array<typeof feedA>; next_cursor: string | null }>;
    error: unknown;
    isLoading: boolean;
    isValidating: boolean;
    setSize: Mock;
    mutate: Mock;
  };

  beforeEach(() => {
    swrState = {
      data: [
        {
          data: [feedA, feedB, feedC],
          next_cursor: null,
        },
      ],
      error: undefined,
      isLoading: false,
      isValidating: false,
      setSize: vi.fn(),
      mutate: vi.fn(async () => {
        swrState.data = [
          {
            data: [feedB, feedC, feedD],
            next_cursor: null,
          },
        ];
        return swrState.data;
      }),
    };

    mockUseSWRInfinite.mockReset();
    mockUseSWRInfinite.mockImplementation(() => swrState);

    mockUpdateFeedReadStatus.mockReset();
    mockUpdateFeedReadStatus.mockResolvedValue({});

    mockTriggerPrefetch.mockReset();
    mockGetCachedContent.mockReset();
    mockMarkAsDismissed.mockReset();
  });

  it("keeps the next feed active after revalidation", async () => {
    const { result } = renderHook(() => useSwipeFeedController());

    const initialActiveId = swrState.data[0]?.data[0]?.id;
    expect(result.current.activeFeed?.id).toBe(initialActiveId);

    await act(async () => {
      await result.current.dismissActiveFeed(1);
    });

    expect(mockUpdateFeedReadStatus).toHaveBeenCalledWith("https://example.com/feed-1");
    const nextActiveId = swrState.data[0]?.data[0]?.id;
    expect(result.current.activeFeed?.id).toBe(nextActiveId);
  });

  it("marks article as dismissed before API call", async () => {
    const { result } = renderHook(() => useSwipeFeedController());

    await act(async () => {
      await result.current.dismissActiveFeed(1);
    });

    // markAsDismissed should be called before read status update
    expect(mockMarkAsDismissed).toHaveBeenCalledWith("https://example.com/feed-1");
    expect(mockMarkAsDismissed).toHaveBeenCalledBefore(mockUpdateFeedReadStatus);
  });

  it("triggers prefetch on initial render", () => {
    renderHook(() => useSwipeFeedController());

    // Prefetch should be called during initial render via activeIndex useEffect
    expect(mockTriggerPrefetch).toHaveBeenCalled();
  });
});

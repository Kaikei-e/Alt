import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi, type Mock } from "vitest";

import { useSwipeFeedController } from "@/components/mobile/feeds/swipe/useSwipeFeedController";

const { mockUseSWRInfinite, mockUpdateFeedReadStatus } = vi.hoisted(() => ({
  mockUseSWRInfinite: vi.fn(),
  mockUpdateFeedReadStatus: vi.fn(),
}));

vi.mock("swr/infinite", () => ({
  default: (...args: unknown[]) => mockUseSWRInfinite(...args),
  useSWRInfinite: (...args: unknown[]) => mockUseSWRInfinite(...args),
}));

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal();
  return {
    ...actual,
    feedsApi: {
      ...actual.feedsApi,
      getFeedsWithCursor: vi.fn(),
      updateFeedReadStatus: mockUpdateFeedReadStatus,
    },
  };
});

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
  });

  it("keeps the next feed active after revalidation", async () => {
    const { result } = renderHook(() => useSwipeFeedController());

    expect(result.current.activeFeed?.id).toBe("feed-1");

    await act(async () => {
      await result.current.dismissActiveFeed(1);
    });

    expect(mockUpdateFeedReadStatus).toHaveBeenCalledWith(
      "https://example.com/feed-1",
    );
    expect(result.current.activeFeed?.id).toBe("feed-2");
  });
});

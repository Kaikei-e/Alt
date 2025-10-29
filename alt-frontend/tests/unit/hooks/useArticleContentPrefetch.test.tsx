import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { useArticleContentPrefetch } from "@/hooks/useArticleContentPrefetch";
import { feedsApi } from "@/lib/api";
import type { Feed } from "@/schema/feed";

// Mock the API
vi.mock("@/lib/api", () => ({
  feedsApi: {
    getFeedContentOnTheFly: vi.fn(),
    archiveContent: vi.fn(),
  },
}));

describe("useArticleContentPrefetch", () => {
  const mockFeeds: Feed[] = [
    {
      id: "1",
      title: "Article 1",
      description: "Description 1",
      link: "https://example.com/article1",
      published: "2025-01-01T00:00:00Z",
    },
    {
      id: "2",
      title: "Article 2",
      description: "Description 2",
      link: "https://example.com/article2",
      published: "2025-01-02T00:00:00Z",
    },
    {
      id: "3",
      title: "Article 3",
      description: "Description 3",
      link: "https://example.com/article3",
      published: "2025-01-03T00:00:00Z",
    },
    {
      id: "4",
      title: "Article 4",
      description: "Description 4",
      link: "https://example.com/article4",
      published: "2025-01-04T00:00:00Z",
    },
    {
      id: "5",
      title: "Article 5",
      description: "Description 5",
      link: "https://example.com/article5",
      published: "2025-01-05T00:00:00Z",
    },
    {
      id: "6",
      title: "Article 6",
      description: "Description 6",
      link: "https://example.com/article6",
      published: "2025-01-06T00:00:00Z",
    },
  ];

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("should initialize with empty cache", () => {
    const { result } = renderHook(() =>
      useArticleContentPrefetch(mockFeeds, 0),
    );

    const cachedContent = result.current.getCachedContent(mockFeeds[0].link);
    expect(cachedContent).toBeNull();
  });

  it("should prefetch next 2 articles when triggered", async () => {
    vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
      content: "<p>Article content</p>",
    });
    vi.mocked(feedsApi.archiveContent).mockResolvedValue({
      message: "archived",
    });

    const { result } = renderHook(() =>
      useArticleContentPrefetch(mockFeeds, 0, 2),
    );

    result.current.triggerPrefetch();

    // Wait for prefetch to complete (real timers)
    await waitFor(
      () => {
        expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalledTimes(2);
      },
      { timeout: 5000 },
    );

    expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalledWith({
      feed_url: "https://example.com/article2",
    });
    expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalledWith({
      feed_url: "https://example.com/article3",
    });
  });

  it("should prefetch next 3 articles when prefetchAhead is 3", async () => {
    vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
      content: "<p>Article content</p>",
    });
    vi.mocked(feedsApi.archiveContent).mockResolvedValue({
      message: "archived",
    });

    const { result } = renderHook(() =>
      useArticleContentPrefetch(mockFeeds, 0, 3),
    );

    result.current.triggerPrefetch();

    await waitFor(
      () => {
        expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalledTimes(3);
      },
      { timeout: 5000 },
    );

    expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalledWith({
      feed_url: "https://example.com/article2",
    });
    expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalledWith({
      feed_url: "https://example.com/article3",
    });
    expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalledWith({
      feed_url: "https://example.com/article4",
    });
  });

  it("should return cached content when available", async () => {
    const mockContent = "<p>Cached article content</p>";
    vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
      content: mockContent,
    });
    vi.mocked(feedsApi.archiveContent).mockResolvedValue({
      message: "archived",
    });

    const { result } = renderHook(() =>
      useArticleContentPrefetch(mockFeeds, 0, 2),
    );

    result.current.triggerPrefetch();

    await waitFor(
      () => {
        const cachedContent = result.current.getCachedContent(
          "https://example.com/article2",
        );
        expect(cachedContent).toBe(mockContent);
      },
      { timeout: 5000 },
    );
  });

  it("should handle prefetch errors gracefully without crashing", async () => {
    const consoleWarnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    vi.mocked(feedsApi.getFeedContentOnTheFly).mockRejectedValue(
      new Error("Network error"),
    );

    const { result } = renderHook(() =>
      useArticleContentPrefetch(mockFeeds, 0, 2),
    );

    result.current.triggerPrefetch();

    // Wait a bit for prefetch to attempt and fail
    await new Promise((resolve) => setTimeout(resolve, 1500));

    // Should not throw error
    expect(() => result.current.getCachedContent(mockFeeds[1].link)).not.toThrow();

    // Cache should not contain failed prefetch
    const cachedContent = result.current.getCachedContent(mockFeeds[1].link);
    expect(cachedContent).toBeNull();

    consoleWarnSpy.mockRestore();
  });

  it("should not prefetch duplicate URLs", async () => {
    vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
      content: "<p>Content</p>",
    });
    vi.mocked(feedsApi.archiveContent).mockResolvedValue({
      message: "archived",
    });

    const { result } = renderHook(() =>
      useArticleContentPrefetch(mockFeeds, 0, 2),
    );

    // Trigger prefetch once
    result.current.triggerPrefetch();

    // Wait for prefetch to complete
    await waitFor(
      () => {
        expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalledTimes(2);
      },
      { timeout: 5000 },
    );

    // Trigger again - should not fetch duplicates
    const callCountBefore = vi.mocked(feedsApi.getFeedContentOnTheFly).mock.calls.length;
    result.current.triggerPrefetch();

    // Wait a bit
    await new Promise((resolve) => setTimeout(resolve, 1500));

    // Should still be 2 calls (no new calls)
    expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalledTimes(callCountBefore);
  });

  it("should clean up cache when size exceeds limit", async () => {
    vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
      content: "<p>Article content</p>",
    });
    vi.mocked(feedsApi.archiveContent).mockResolvedValue({
      message: "archived",
    });

    const { result, rerender } = renderHook(
      ({ feeds, activeIndex }) => useArticleContentPrefetch(feeds, activeIndex, 2),
      {
        initialProps: { feeds: mockFeeds, activeIndex: 0 },
      },
    );

    // Prefetch first 2 articles
    result.current.triggerPrefetch();
    await waitFor(
      () => {
        expect(result.current.contentCacheRef.current.size).toBeGreaterThanOrEqual(2);
      },
      { timeout: 5000 },
    );

    // Move to next index and prefetch more
    rerender({ feeds: mockFeeds, activeIndex: 1 });
    result.current.triggerPrefetch();
    await waitFor(
      () => {
        expect(result.current.contentCacheRef.current.size).toBeGreaterThanOrEqual(3);
      },
      { timeout: 5000 },
    );

    // Move to next index and prefetch more (should trigger cleanup)
    rerender({ feeds: mockFeeds, activeIndex: 2 });
    result.current.triggerPrefetch();
    await waitFor(
      () => {
        // Cache should be limited to 5 entries
        expect(result.current.contentCacheRef.current.size).toBeLessThanOrEqual(5);
        expect(result.current.contentCacheRef.current.size).toBeGreaterThan(0);
      },
      { timeout: 5000 },
    );
  });

  it("should archive articles in background non-blocking", async () => {
    vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
      content: "<p>Article content</p>",
    });
    vi.mocked(feedsApi.archiveContent).mockResolvedValue({
      message: "archived",
    });

    const { result } = renderHook(() =>
      useArticleContentPrefetch(mockFeeds, 0, 2),
    );

    result.current.triggerPrefetch();

    // Wait for archive to be called (at least once)
    await waitFor(
      () => {
        expect(feedsApi.archiveContent).toHaveBeenCalled();
      },
      { timeout: 5000 },
    );

    // Wait a bit longer for the second archive
    await new Promise((resolve) => setTimeout(resolve, 1000));

    // Check that articles were archived
    expect(feedsApi.archiveContent).toHaveBeenCalledWith(
      expect.stringContaining("example.com/article"),
      expect.any(String),
    );
  });

  it("should not block on archive failures", async () => {
    const consoleWarnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
      content: "<p>Article content</p>",
    });
    vi.mocked(feedsApi.archiveContent).mockRejectedValue(
      new Error("Archive failed"),
    );

    const { result } = renderHook(() =>
      useArticleContentPrefetch(mockFeeds, 0, 2),
    );

    result.current.triggerPrefetch();

    // Content should still be cached despite archive failure
    await waitFor(
      () => {
        const cachedContent = result.current.getCachedContent(
          "https://example.com/article2",
        );
        expect(cachedContent).toBe("<p>Article content</p>");
      },
      { timeout: 5000 },
    );

    consoleWarnSpy.mockRestore();
  });

  it("should handle edge case when no more articles to prefetch", async () => {
    // Create an empty feed list
    const emptyFeedList: Feed[] = [];

    const { result } = renderHook(() =>
      useArticleContentPrefetch(emptyFeedList, 0, 2),
    );

    // Should not throw when triggering prefetch with empty list
    expect(() => result.current.triggerPrefetch()).not.toThrow();

    // Wait a bit
    await new Promise((resolve) => setTimeout(resolve, 500));

    // Should not have any cached content
    expect(result.current.contentCacheRef.current.size).toBe(0);
  });

  it("should return null for loading content", async () => {
    vi.mocked(feedsApi.getFeedContentOnTheFly).mockImplementation(
      () =>
        new Promise((resolve) => {
          setTimeout(() => resolve({ content: "<p>Content</p>" }), 5000);
        }),
    );

    const { result } = renderHook(() =>
      useArticleContentPrefetch(mockFeeds, 0, 1),
    );

    result.current.triggerPrefetch();

    // Wait for timeout to be triggered but not for response
    await new Promise((resolve) => setTimeout(resolve, 600));

    // Content should be in "loading" state
    const cachedContent = result.current.getCachedContent(mockFeeds[1].link);
    expect(cachedContent).toBeNull();
  });
});

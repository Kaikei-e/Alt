import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import SwipeFeedCard from "@/components/mobile/feeds/swipe/SwipeFeedCard";
import type { Feed } from "@/schema/feed";

vi.mock("@/lib/api", () => ({
  feedsApi: {
    getFeedContentOnTheFly: vi.fn(),
    archiveContent: vi.fn(),
    getArticleSummary: vi.fn(),
    summarizeArticle: vi.fn(),
  },
}));

vi.mock("framer-motion", () => ({
  motion: {
    div: ({ children, ...props }: any) => {
      // propsからmotion固有のpropsを除外して通常のdivとしてレンダリング
      const { initial, animate, exit, style, ...restProps } = props;
      return (
        <div {...restProps} style={style}>
          {children}
        </div>
      );
    },
  },
  AnimatePresence: ({ children, ...props }: any) => {
    // childrenを確実にレンダリング
    // AnimatePresenceは単純にchildrenを返すだけにする
    if (Array.isArray(children)) {
      return <>{children}</>;
    }
    return <>{children}</>;
  },
  useMotionValue: (initial: number = 0) => {
    const value = { current: initial };
    return {
      set: (newValue: number) => {
        value.current = newValue;
      },
      get: () => value.current,
      current: value.current,
    };
  },
  animate: vi.fn(),
}));

vi.mock("@use-gesture/react", () => ({
  useDrag: () => () => ({}),
}));

const baseFeed: Feed = {
  id: "feed-1",
  title: "Test Feed",
  link: "https://example.com/feed-1",
  description: "Feed description",
  published: new Date().toISOString(),
};

const renderCard = (
  feed: Feed = baseFeed,
  overrides: Partial<React.ComponentProps<typeof SwipeFeedCard>> = {}
) => {
  return render(
    <ChakraProvider value={defaultSystem}>
      <SwipeFeedCard
        feed={feed}
        statusMessage={overrides.statusMessage ?? null}
        onDismiss={overrides.onDismiss ?? vi.fn()}
        getCachedContent={overrides.getCachedContent}
      />
    </ChakraProvider>
  );
};

describe("SwipeFeedCard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("fetches and displays summary on first expand", async () => {
    const { feedsApi } = await import("@/lib/api");
    vi.mocked(feedsApi.getArticleSummary).mockResolvedValue({
      matched_articles: [
        {
          article_url: baseFeed.link,
          title: baseFeed.title,
          content: "これは要約です",
          content_type: "summary",
          published_at: baseFeed.published,
          fetched_at: new Date().toISOString(),
          source_id: baseFeed.id,
        },
      ],
      total_matched: 1,
      requested_count: 1,
    });

    renderCard();

    // Wait for component to fully render with AnimatePresence
    // MotionBoxのレンダリングを待つため、より長いタイムアウトを使用
    await waitFor(
      () => {
        expect(screen.queryByTestId("swipe-card")).toBeInTheDocument();
      },
      { timeout: 3000 }
    );
    const actionFooter = await screen.findByTestId("action-footer", {}, { timeout: 3000 });
    const summaryToggle = within(actionFooter).getByTestId("toggle-summary-button");
    await act(async () => {
      fireEvent.click(summaryToggle);
    });
    await waitFor(() => {
      expect(screen.queryAllByTestId("summary-section").length).toBeGreaterThan(0);
    });
    const summarySection = screen.getAllByTestId("summary-section")[0];
    expect(summarySection).toHaveTextContent("これは要約です");
  });

  it("fetches full content and archives article when expanded", async () => {
    const { feedsApi } = await import("@/lib/api");
    vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
      content: "<p>Full article</p>",
    });
    vi.mocked(feedsApi.archiveContent).mockResolvedValue({ message: "ok" });

    renderCard();

    // Wait for component to fully render
    await waitFor(
      () => {
        expect(screen.queryByTestId("swipe-card")).toBeInTheDocument();
      },
      { timeout: 3000 }
    );
    const actionFooter = await screen.findByTestId("action-footer", {}, { timeout: 3000 });
    const contentToggle = within(actionFooter).getByTestId("toggle-content-button");
    fireEvent.click(contentToggle);

    await waitFor(() =>
      expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalledWith({
        feed_url: baseFeed.link,
      })
    );
    await waitFor(() =>
      expect(feedsApi.archiveContent).toHaveBeenCalledWith(baseFeed.link, baseFeed.title)
    );
    const contentSection = await screen.findByTestId("content-section");
    expect(contentSection).toHaveTextContent("Full article");
  });

  it("shows status message when provided", async () => {
    renderCard(baseFeed, { statusMessage: "Feed marked as read" });

    // Wait for action-footer to render first, then check for status message
    await waitFor(
      () => {
        expect(screen.queryByTestId("swipe-card")).toBeInTheDocument();
      },
      { timeout: 3000 }
    );
    await screen.findByTestId("action-footer", {}, { timeout: 3000 });

    await waitFor(
      () => {
        expect(screen.getByText("Feed marked as read")).toBeInTheDocument();
      },
      { timeout: 3000 }
    );
  });

  it("resets expanded state when feed changes", async () => {
    const { feedsApi } = await import("@/lib/api");
    vi.mocked(feedsApi.getArticleSummary).mockResolvedValue({
      matched_articles: [
        {
          article_url: baseFeed.link,
          title: baseFeed.title,
          content: "summary",
          content_type: "summary",
          published_at: baseFeed.published,
          fetched_at: new Date().toISOString(),
          source_id: baseFeed.id,
        },
      ],
      total_matched: 1,
      requested_count: 1,
    });

    const { rerender } = renderCard();

    // Wait for component to fully render
    await waitFor(
      () => {
        expect(screen.queryByTestId("swipe-card")).toBeInTheDocument();
      },
      { timeout: 3000 }
    );
    const actionFooter = await screen.findByTestId("action-footer", {}, { timeout: 3000 });
    const summaryToggle = within(actionFooter).getByTestId("toggle-summary-button");
    await act(async () => {
      fireEvent.click(summaryToggle);
    });
    const nextFeed = {
      ...baseFeed,
      id: "feed-2",
      link: "https://example.com/feed-2",
      title: "Next feed",
    };
    rerender(
      <ChakraProvider value={defaultSystem}>
        <SwipeFeedCard feed={nextFeed} statusMessage={null} onDismiss={vi.fn()} />
      </ChakraProvider>
    );

    expect(screen.queryByTestId("summary-section")).not.toBeInTheDocument();
  });

  it("uses cached content when available instead of fetching", async () => {
    const { feedsApi } = await import("@/lib/api");
    const cachedContent = "<p>Cached article content</p>";
    const getCachedContent = vi.fn().mockReturnValue(cachedContent);

    renderCard(baseFeed, { getCachedContent });

    // Wait for component to fully render
    const actionFooter = await screen.findByTestId("action-footer", {}, { timeout: 3000 });
    const contentToggle = within(actionFooter).getByTestId("toggle-content-button");

    await act(async () => {
      fireEvent.click(contentToggle);
    });

    // getCachedContent should have been called
    expect(getCachedContent).toHaveBeenCalledWith(baseFeed.link);

    // API should not have been called since we had cached content
    expect(feedsApi.getFeedContentOnTheFly).not.toHaveBeenCalled();

    // Content should be displayed
    const contentSection = await screen.findByTestId("content-section");
    expect(contentSection).toHaveTextContent("Cached article content");
  });

  it("falls back to fetching when cache miss occurs", async () => {
    const { feedsApi } = await import("@/lib/api");
    const getCachedContent = vi.fn().mockReturnValue(null); // Cache miss
    vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
      content: "<p>Fetched article content</p>",
    });
    vi.mocked(feedsApi.archiveContent).mockResolvedValue({ message: "ok" });

    renderCard(baseFeed, { getCachedContent });

    // Wait for component to fully render
    await waitFor(
      () => {
        expect(screen.queryByTestId("swipe-card")).toBeInTheDocument();
      },
      { timeout: 3000 }
    );
    const actionFooter = await screen.findByTestId("action-footer", {}, { timeout: 3000 });
    const contentToggle = within(actionFooter).getByTestId("toggle-content-button");

    await act(async () => {
      fireEvent.click(contentToggle);
    });

    // getCachedContent should have been called
    expect(getCachedContent).toHaveBeenCalledWith(baseFeed.link);

    // Since cache missed, API should have been called
    await waitFor(() => {
      expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalledWith({
        feed_url: baseFeed.link,
      });
    });

    const contentSection = await screen.findByTestId("content-section");
    expect(contentSection).toHaveTextContent("Fetched article content");
  });

  it("does not call getCachedContent when prop is not provided", async () => {
    const { feedsApi } = await import("@/lib/api");
    vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
      content: "<p>Fetched content</p>",
    });
    vi.mocked(feedsApi.archiveContent).mockResolvedValue({ message: "ok" });

    renderCard(baseFeed); // No getCachedContent prop

    // Wait for component to fully render
    await waitFor(
      () => {
        expect(screen.queryByTestId("swipe-card")).toBeInTheDocument();
      },
      { timeout: 3000 }
    );
    const actionFooter = await screen.findByTestId("action-footer", {}, { timeout: 3000 });
    const contentToggle = within(actionFooter).getByTestId("toggle-content-button");
    fireEvent.click(contentToggle);

    await waitFor(() => {
      expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalledWith({
        feed_url: baseFeed.link,
      });
    });
  });
});

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import SwipeFeedCard from "@/components/mobile/feeds/swipe/SwipeFeedCard";
import { Feed } from "@/schema/feed";

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
    div: ({ children, ...props }: any) => <div {...props}>{children}</div>,
  },
  AnimatePresence: ({ children }: any) => <>{children}</>,
  useMotionValue: () => ({
    set: vi.fn(),
    get: () => 0,
  }),
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
  feed_url: "https://example.com/feed-1",
};

const renderCard = (
  feed: Feed = baseFeed,
  overrides: Partial<React.ComponentProps<typeof SwipeFeedCard>> = {},
) => {
  return render(
    <ChakraProvider value={defaultSystem}>
      <SwipeFeedCard
        feed={feed}
        statusMessage={overrides.statusMessage ?? null}
        onDismiss={overrides.onDismiss ?? vi.fn()}
      />
    </ChakraProvider>,
  );
};

describe("SwipeFeedCard", () => {
  let user: ReturnType<typeof userEvent.setup>;

  beforeEach(() => {
    user = userEvent.setup();
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("fetches and displays summary on first expand", async () => {
    const { feedsApi } = await import("@/lib/api");
    vi.mocked(feedsApi.getArticleSummary).mockResolvedValue({
      matched_articles: [{ content: "これは要約です" }],
    });

    renderCard();

    await user.click(screen.getByTestId("toggle-summary-button"));
    await waitFor(() =>
      expect(feedsApi.getArticleSummary).toHaveBeenCalledWith(baseFeed.link),
    );

    const summarySection = await screen.findByTestId("summary-section");
    expect(summarySection).toHaveTextContent("これは要約です");
  });

  it("fetches full content and archives article when expanded", async () => {
    const { feedsApi } = await import("@/lib/api");
    vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
      content: "<p>Full article</p>",
    });
    vi.mocked(feedsApi.archiveContent).mockResolvedValue({ message: "ok" });

    renderCard();

    await user.click(screen.getByTestId("toggle-content-button"));

    await waitFor(() =>
      expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalledWith({
        feed_url: baseFeed.link,
      }),
    );
    await waitFor(() =>
      expect(feedsApi.archiveContent).toHaveBeenCalledWith(
        baseFeed.link,
        baseFeed.title,
      ),
    );
    const contentSection = await screen.findByTestId("content-section");
    expect(contentSection).toHaveTextContent("Full article");
  });

  it("invokes dismiss callback after mark as read button is pressed", async () => {
    const onDismiss = vi.fn().mockResolvedValue(undefined);
    renderCard(baseFeed, { onDismiss, statusMessage: "status" });

    await user.click(screen.getByTestId("swipe-card-button"));

    await waitFor(() => {
      expect(onDismiss).toHaveBeenCalled();
    });
  });

  it("resets expanded state when feed changes", async () => {
    const { feedsApi } = await import("@/lib/api");
    vi.mocked(feedsApi.getArticleSummary).mockResolvedValue({
      matched_articles: [{ content: "summary" }],
    });

    const { rerender } = renderCard();

    await user.click(screen.getByTestId("toggle-summary-button"));
    await waitFor(() => expect(screen.getByText("summary")).toBeInTheDocument());

    const nextFeed = {
      ...baseFeed,
      id: "feed-2",
      link: "https://example.com/feed-2",
      title: "Next feed",
    };
    rerender(
      <ChakraProvider value={defaultSystem}>
        <SwipeFeedCard feed={nextFeed} statusMessage={null} onDismiss={vi.fn()} />
      </ChakraProvider>,
    );

    expect(screen.queryByTestId("summary-section")).not.toBeInTheDocument();
  });
});

import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent, within } from "@testing-library/react";
import { act } from "react";
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
  beforeEach(() => {
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

    await waitFor(() => {
      expect(screen.getAllByTestId("action-footer")[0]).toBeDefined();
    });
    const actionFooter = screen.getAllByTestId("action-footer")[0];
    const summaryToggle = within(actionFooter).getByTestId(
      "toggle-summary-button",
    );
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

    await waitFor(() => {
      expect(screen.getAllByTestId("action-footer")[0]).toBeDefined();
    });
    const actionFooter = screen.getAllByTestId("action-footer")[0];
    const contentToggle = within(actionFooter).getByTestId(
      "toggle-content-button",
    );
    fireEvent.click(contentToggle);

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

  it("shows status message when provided", () => {
    renderCard(baseFeed, { statusMessage: "Feed marked as read" });

    expect(screen.getByText("Feed marked as read")).toBeInTheDocument();
  });

  it("resets expanded state when feed changes", async () => {
    const { feedsApi } = await import("@/lib/api");
    vi.mocked(feedsApi.getArticleSummary).mockResolvedValue({
      matched_articles: [{ content: "summary" }],
    });

    const { rerender } = renderCard();

    await waitFor(() => {
      expect(screen.getAllByTestId("action-footer")[0]).toBeDefined();
    });
    const actionFooter = screen.getAllByTestId("action-footer")[0];
    const summaryToggle = within(actionFooter).getByTestId(
      "toggle-summary-button",
    );
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
      </ChakraProvider>,
    );

    expect(screen.queryByTestId("summary-section")).not.toBeInTheDocument();
  });
});

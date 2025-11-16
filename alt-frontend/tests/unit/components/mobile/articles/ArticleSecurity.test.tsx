import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type React from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ArticleDetailsModal } from "@/components/mobile/articles/ArticleDetailsModal";
import SwipeFeedCard from "@/components/mobile/feeds/swipe/SwipeFeedCard";
import { articleApi } from "@/lib/api";
import type { Article } from "@/schema/article";
import type { Feed } from "@/schema/feed";
import "../test-env";

vi.mock("@/lib/api", () => ({
  articleApi: {
    getFeedContentOnTheFly: vi.fn(),
    getArticleSummary: vi.fn(),
    summarizeArticle: vi.fn(),
    archiveContent: vi.fn(),
  },
  feedsApi: {
    getFeedContentOnTheFly: vi.fn(),
    getArticleSummary: vi.fn(),
    summarizeArticle: vi.fn(),
    archiveContent: vi.fn(),
    registerFavoriteFeed: vi.fn(),
  },
}));

const renderWithProviders = (ui: React.ReactElement) =>
  render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);

const setupUser = () => userEvent.setup({ pointerEventsCheck: 0 });

describe("Article rendering security", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("sanitizes malicious HTML in ArticleDetailsModal full content", async () => {
    const article: Article = {
      id: "article-1",
      title: "Example article",
      content: "Short preview",
      url: "https://example.com/article",
      published_at: new Date().toISOString(),
    };

    const maliciousContent =
      "<p>hello</p><script>window.__xss = true;</script>";

    vi.mocked(articleApi.getFeedContentOnTheFly).mockResolvedValue({
      content: maliciousContent,
    });

    renderWithProviders(
      <ArticleDetailsModal article={article} isOpen onClose={() => { }} />,
    );

    const contentNode = await screen.findByTestId("article-full-content");

    await waitFor(() => {
      expect(articleApi.getFeedContentOnTheFly).toHaveBeenCalledWith({
        feed_url: article.url,
      });
    });

    expect(contentNode.querySelector("script")).toBeNull();
    expect(contentNode.innerHTML).not.toContain("<script");
  });

  it("sanitizes malicious HTML in SwipeFeedCard expanded content", async () => {
    const user = setupUser();

    const feed: Feed = {
      id: "feed-1",
      title: "Example feed",
      link: "https://example.com/feed", // link used for fetching full content
      author: "Author",
      description: "Feed description",
      published: new Date().toISOString(),
    };

    const maliciousContent =
      "<div>content</div><script>window.__cardXss = true;</script>";

    vi.mocked(articleApi.getFeedContentOnTheFly).mockResolvedValue({
      content: maliciousContent,
    });

    vi.mocked(articleApi.archiveContent).mockResolvedValue({
      message: "archived",
    });

    renderWithProviders(
      <SwipeFeedCard
        feed={feed}
        statusMessage={null}
        onDismiss={vi.fn()}
      />,
    );

    const toggleContentButton = screen.getByTestId("toggle-content-button");
    await user.click(toggleContentButton);

    await waitFor(
      () => {
        expect(articleApi.getFeedContentOnTheFly).toHaveBeenCalledWith({
          feed_url: feed.link,
        });
      },
      { timeout: 5000 },
    );

    await waitFor(() => {
      const contentSection = screen.getByTestId("content-section");
      expect(contentSection).toBeInTheDocument();
    });

    const contentSection = screen.getByTestId("content-section");

    expect(contentSection.querySelector("script")).toBeNull();
    expect(contentSection.innerHTML).not.toContain("<script");
  });
});

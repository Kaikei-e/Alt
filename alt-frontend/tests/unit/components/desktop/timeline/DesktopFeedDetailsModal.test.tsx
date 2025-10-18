import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api", () => ({
  feedsApi: {
    getFeedContentOnTheFly: vi.fn(),
    getArticleSummary: vi.fn(),
    archiveContent: vi.fn(),
    registerFavoriteFeed: vi.fn(),
    summarizeArticle: vi.fn(),
  },
}));

import { feedsApi as mockedFeedsApi } from "@/lib/api";
import { DesktopFeedDetailsModal } from "@/components/desktop/timeline/DesktopFeedDetailsModal";

type MockFeedsApi = {
  getFeedContentOnTheFly: ReturnType<typeof vi.fn>;
  getArticleSummary: ReturnType<typeof vi.fn>;
  archiveContent: ReturnType<typeof vi.fn>;
  registerFavoriteFeed: ReturnType<typeof vi.fn>;
  summarizeArticle: ReturnType<typeof vi.fn>;
};

const mockFeedsApi = mockedFeedsApi as unknown as MockFeedsApi;

const renderWithChakra = (ui: React.ReactElement) =>
  render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);

describe("DesktopFeedDetailsModal", () => {
  const feedLink = "https://example.com/article";
  const feedTitle = "Example Article";

  beforeEach(() => {
    mockFeedsApi.getFeedContentOnTheFly.mockReset();
    mockFeedsApi.getArticleSummary.mockReset();
    mockFeedsApi.archiveContent.mockReset();
    mockFeedsApi.registerFavoriteFeed.mockReset();
    mockFeedsApi.summarizeArticle.mockReset();
  });

  it("renders header link and article content when opened", async () => {
    mockFeedsApi.getFeedContentOnTheFly.mockResolvedValue({
      content: "<p>Full article content</p>",
    });
    mockFeedsApi.getArticleSummary.mockResolvedValue({
      matched_articles: [],
      total_matched: 0,
      requested_count: 1,
    });
    mockFeedsApi.archiveContent.mockResolvedValue({ message: "ok" });

    renderWithChakra(
      <DesktopFeedDetailsModal
        isOpen
        onClose={() => {}}
        feedLink={feedLink}
        feedTitle={feedTitle}
        feedId="feed-1"
      />,
    );

    await waitFor(() =>
      expect(mockFeedsApi.getFeedContentOnTheFly).toHaveBeenCalledWith({
        feed_url: feedLink,
      }),
    );

    const headerLink = await screen.findByRole("link", {
      name: feedTitle,
    });
    expect(headerLink).toHaveAttribute("href", feedLink);

    await waitFor(() =>
      expect(screen.getByText("Full article content")).toBeInTheDocument(),
    );

    expect(
      screen.getByTestId("desktop-feed-details-archive-feed-1"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("desktop-feed-details-ai-feed-1"))
      .toBeInTheDocument();
  });

  it("triggers API actions from footer controls", async () => {
    mockFeedsApi.getFeedContentOnTheFly.mockResolvedValue({
      content: "<p>Full article content</p>",
    });
    mockFeedsApi.getArticleSummary.mockResolvedValue({
      matched_articles: [],
      total_matched: 0,
      requested_count: 1,
    });
    mockFeedsApi.archiveContent.mockResolvedValue({ message: "ok" });
    mockFeedsApi.summarizeArticle.mockResolvedValue({
      summary: "AI generated summary",
    });

    renderWithChakra(
      <DesktopFeedDetailsModal
        isOpen
        onClose={() => {}}
        feedLink={feedLink}
        feedTitle={feedTitle}
        feedId="feed-1"
      />,
    );

    const user = userEvent.setup();

    const archiveButton = await screen.findByTestId(
      "desktop-feed-details-archive-feed-1",
    );
    await user.click(archiveButton);
    await waitFor(() =>
      expect(mockFeedsApi.archiveContent).toHaveBeenCalledWith(
        feedLink,
        feedTitle,
      ),
    );

    const summarizeButton = screen.getByTestId(
      "desktop-feed-details-ai-feed-1",
    );
    await user.click(summarizeButton);
    await waitFor(() =>
      expect(mockFeedsApi.summarizeArticle).toHaveBeenCalledWith(feedLink),
    );

    await waitFor(() =>
      expect(screen.getByText("AI generated summary")).toBeInTheDocument(),
    );
  });
});

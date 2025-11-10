import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/api", () => ({
  articleApi: {
    getFeedContentOnTheFly: vi.fn(),
    getArticleSummary: vi.fn(),
    archiveContent: vi.fn(),
    summarizeArticle: vi.fn(),
  },
  feedsApi: {
    getFeedContentOnTheFly: vi.fn(),
    getArticleSummary: vi.fn(),
    archiveContent: vi.fn(),
    registerFavoriteFeed: vi.fn(),
    summarizeArticle: vi.fn(),
  },
}));

import { DesktopFeedDetailsModal } from "@/components/desktop/timeline/DesktopFeedDetailsModal";
import { articleApi as mockedArticleApi } from "@/lib/api";

type MockArticleApi = {
  getFeedContentOnTheFly: ReturnType<typeof vi.fn>;
  getArticleSummary: ReturnType<typeof vi.fn>;
  archiveContent: ReturnType<typeof vi.fn>;
  summarizeArticle: ReturnType<typeof vi.fn>;
};

const mockArticleApi = mockedArticleApi as unknown as MockArticleApi;

const renderWithChakra = (ui: React.ReactElement) =>
  render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);

const findActiveModal = async (): Promise<HTMLElement> => {
  const modals = await screen.findAllByTestId("desktop-feed-details-modal-feed-1");

  const activeModal =
    modals.find((modal) => modal.getAttribute("data-state") === "open") ??
    modals[modals.length - 1];

  if (!activeModal) {
    throw new Error("Desktop feed details modal not found");
  }

  return activeModal;
};

describe("DesktopFeedDetailsModal", () => {
  const feedLink = "https://example.com/article";
  const feedTitle = "Example Article";

  beforeEach(() => {
    mockArticleApi.getFeedContentOnTheFly.mockReset();
    mockArticleApi.getArticleSummary.mockReset();
    mockArticleApi.archiveContent.mockReset();
    mockArticleApi.summarizeArticle.mockReset();
  });

  it("renders header link and article content when opened", async () => {
    mockArticleApi.getFeedContentOnTheFly.mockResolvedValue({
      content: "<p>Full article content</p>",
    });
    mockArticleApi.getArticleSummary.mockResolvedValue({
      matched_articles: [],
      total_matched: 0,
      requested_count: 1,
    });
    mockArticleApi.archiveContent.mockResolvedValue({ message: "ok" });

    renderWithChakra(
      <DesktopFeedDetailsModal
        isOpen
        onClose={() => { }}
        feedLink={feedLink}
        feedTitle={feedTitle}
        feedId="feed-1"
      />
    );

    await waitFor(() =>
      expect(mockArticleApi.getFeedContentOnTheFly).toHaveBeenCalledWith({
        feed_url: feedLink,
      })
    );

    const headerLink = await screen.findByRole("link", {
      name: feedTitle,
    });
    expect(headerLink).toHaveAttribute("href", feedLink);

    await waitFor(() => expect(screen.getByText("Full article content")).toBeInTheDocument());

    const modal = await findActiveModal();

    expect(
      await within(modal).findByTestId("desktop-feed-details-archive-feed-1")
    ).toBeInTheDocument();
    expect(await within(modal).findByTestId("desktop-feed-details-ai-feed-1")).toBeInTheDocument();
  });

  it("triggers API actions from footer controls", async () => {
    mockArticleApi.getFeedContentOnTheFly.mockResolvedValue({
      content: "<p>Full article content</p>",
    });
    mockArticleApi.getArticleSummary.mockResolvedValue({
      matched_articles: [],
      total_matched: 0,
      requested_count: 1,
    });
    mockArticleApi.archiveContent.mockResolvedValue({ message: "ok" });
    mockArticleApi.summarizeArticle.mockResolvedValue({
      success: true,
      summary: "AI generated summary",
      article_id: "test-id",
      feed_url: feedLink,
    });

    renderWithChakra(
      <DesktopFeedDetailsModal
        isOpen
        onClose={() => { }}
        feedLink={feedLink}
        feedTitle={feedTitle}
        feedId="feed-1"
      />
    );

    const user = userEvent.setup({ pointerEventsCheck: 0 });
    const modal = await findActiveModal();

    const archiveButton = await within(modal).findByTestId("desktop-feed-details-archive-feed-1");
    await user.click(archiveButton);
    await waitFor(() =>
      expect(mockArticleApi.archiveContent).toHaveBeenCalledWith(feedLink, feedTitle)
    );

    const summarizeButton = within(modal).getByTestId("desktop-feed-details-ai-feed-1");
    await user.click(summarizeButton);
    await waitFor(() => expect(mockArticleApi.summarizeArticle).toHaveBeenCalledWith(feedLink));

    await waitFor(() => expect(screen.getByText("AI generated summary")).toBeInTheDocument());
  });
});

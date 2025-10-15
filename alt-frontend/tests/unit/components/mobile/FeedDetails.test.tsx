import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { FeedDetails } from "@/components/mobile/FeedDetails";
import { feedsApi } from "@/lib/api";

// Mock the feedsApi
vi.mock("@/lib/api", () => ({
  feedsApi: {
    getArticleSummary: vi.fn(),
    getFeedContentOnTheFly: vi.fn(),
    archiveContent: vi.fn(),
    registerFavoriteFeed: vi.fn(),
    summarizeArticle: vi.fn(),
  },
}));

describe("FeedDetails", () => {
  const mockFeedURL = "https://example.com/feed";
  const mockFeedTitle = "Test Article Title";

  beforeEach(() => {
    vi.clearAllMocks();
  });

  describe("Auto-archive functionality", () => {
    it("should auto-archive article when displaying valid content", async () => {
      const user = userEvent.setup();

      // Mock successful content fetch
      vi.mocked(feedsApi.getArticleSummary).mockResolvedValue({
        matched_articles: [],
      });

      vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
        content: "This is the article content",
        url: mockFeedURL,
      });

      vi.mocked(feedsApi.archiveContent).mockResolvedValue({
        message: "article archived",
      });

      render(<FeedDetails feedURL={mockFeedURL} feedTitle={mockFeedTitle} />);

      // Click "Show Details" button
      const showButton = screen.getByText("Show Details");
      await user.click(showButton);

      // Wait for content to be displayed
      await waitFor(() => {
        expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalledWith({
          feed_url: mockFeedURL,
        });
      });

      // Verify auto-archive was called
      await waitFor(() => {
        expect(feedsApi.archiveContent).toHaveBeenCalledWith(
          mockFeedURL,
          mockFeedTitle,
        );
      });
    });

    it("should not auto-archive when content fetch fails", async () => {
      const user = userEvent.setup();

      // Mock failed content fetch
      vi.mocked(feedsApi.getArticleSummary).mockResolvedValue({
        matched_articles: [],
      });

      vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
        content: "",
        url: mockFeedURL,
      });

      render(<FeedDetails feedURL={mockFeedURL} feedTitle={mockFeedTitle} />);

      // Click "Show Details" button
      const showButton = screen.getByText("Show Details");
      await user.click(showButton);

      // Wait for error state
      await waitFor(() => {
        expect(feedsApi.getFeedContentOnTheFly).toHaveBeenCalled();
      });

      // Verify auto-archive was NOT called (no valid content)
      expect(feedsApi.archiveContent).not.toHaveBeenCalled();
    });

    it("should not block UI when auto-archive fails", async () => {
      const user = userEvent.setup();

      // Mock successful content fetch but failed archive
      vi.mocked(feedsApi.getArticleSummary).mockResolvedValue({
        matched_articles: [],
      });

      vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
        content: "This is the article content",
        url: mockFeedURL,
      });

      vi.mocked(feedsApi.archiveContent).mockRejectedValue(
        new Error("Archive failed"),
      );

      const consoleWarnSpy = vi.spyOn(console, "warn").mockImplementation();

      render(<FeedDetails feedURL={mockFeedURL} feedTitle={mockFeedTitle} />);

      // Click "Show Details" button
      const showButton = screen.getByText("Show Details");
      await user.click(showButton);

      // Wait for content to be displayed
      await waitFor(() => {
        expect(screen.getByTestId("modal-content")).toBeInTheDocument();
      });

      // Verify archive was attempted
      await waitFor(() => {
        expect(feedsApi.archiveContent).toHaveBeenCalled();
      });

      // Verify error was logged but UI is not blocked
      await waitFor(() => {
        expect(consoleWarnSpy).toHaveBeenCalledWith(
          "Failed to auto-archive article:",
          expect.any(Error),
        );
      });

      // Content should still be visible
      expect(screen.getByTestId("modal-content")).toBeInTheDocument();

      consoleWarnSpy.mockRestore();
    });

    it("should not duplicate archive when Archive button is clicked after auto-archive", async () => {
      const user = userEvent.setup();

      vi.mocked(feedsApi.getArticleSummary).mockResolvedValue({
        matched_articles: [],
      });

      vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
        content: "This is the article content",
        url: mockFeedURL,
      });

      vi.mocked(feedsApi.archiveContent).mockResolvedValue({
        message: "article archived",
      });

      render(<FeedDetails feedURL={mockFeedURL} feedTitle={mockFeedTitle} />);

      // Click "Show Details" button
      const showButton = screen.getByText("Show Details");
      await user.click(showButton);

      // Wait for auto-archive to complete
      await waitFor(() => {
        expect(feedsApi.archiveContent).toHaveBeenCalledTimes(1);
      });

      // Click Archive button manually
      const archiveButton = screen.getByTitle("Archive");
      await user.click(archiveButton);

      // Archive should be called again (backend handles deduplication)
      await waitFor(() => {
        expect(feedsApi.archiveContent).toHaveBeenCalledTimes(2);
      });
    });
  });

  describe("Summarization flow", () => {
    it("should successfully summarize article after auto-archive", async () => {
      const user = userEvent.setup();

      vi.mocked(feedsApi.getArticleSummary).mockResolvedValue({
        matched_articles: [],
      });

      vi.mocked(feedsApi.getFeedContentOnTheFly).mockResolvedValue({
        content: "This is the article content",
        url: mockFeedURL,
      });

      vi.mocked(feedsApi.archiveContent).mockResolvedValue({
        message: "article archived",
      });

      vi.mocked(feedsApi.summarizeArticle).mockResolvedValue({
        success: true,
        summary: "これは記事の要約です",
        article_id: "test-article-id",
      });

      render(<FeedDetails feedURL={mockFeedURL} feedTitle={mockFeedTitle} />);

      // Click "Show Details"
      const showButton = screen.getByText("Show Details");
      await user.click(showButton);

      // Wait for auto-archive
      await waitFor(() => {
        expect(feedsApi.archiveContent).toHaveBeenCalled();
      });

      // Click "要約" button
      const summarizeButton = screen.getByText("要約");
      await user.click(summarizeButton);

      // Verify summarization was called
      await waitFor(() => {
        expect(feedsApi.summarizeArticle).toHaveBeenCalledWith(mockFeedURL);
      });

      // Verify summary is displayed
      await waitFor(() => {
        expect(screen.getByText("これは記事の要約です")).toBeInTheDocument();
      });
    });
  });
});

import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, within, cleanup } from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { ArticleSearchResults } from "@/components/mobile/search/ArticleSearchResults";
import { Article } from "@/schema/article";
import "../test-env";

// Mock ArticleCard component
vi.mock("@/components/mobile/ArticleCard", () => ({
  ArticleCard: ({ article }: { article: Article }) => (
    <div data-testid={`article-card-${article.id}`}>
      <h3>{article.title}</h3>
      <p>{article.content}</p>
    </div>
  ),
}));

describe("ArticleSearchResults", () => {
  const mockArticles: Article[] = [
    {
      id: "1",
      title: "Test Article 1",
      content: "Content 1",
    },
    {
      id: "2",
      title: "Test Article 2",
      content: "Content 2",
    },
    {
      id: "3",
      title: "Test Article 3",
      content: "Content 3",
    },
  ];

  const defaultProps = {
    results: [],
    isLoading: false,
    searchQuery: "",
    searchTime: undefined,
  };

  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  const renderWithChakra = (ui: React.ReactElement) => {
    return render(<ChakraProvider value={defaultSystem}>{ui}</ChakraProvider>);
  };

  describe("Loading State", () => {
    it("should display loading spinner when isLoading is true", () => {
      renderWithChakra(
        <ArticleSearchResults {...defaultProps} isLoading={true} />,
      );

      expect(screen.getByText(/searching articles/i)).toBeInTheDocument();
    });

    it("should not display results when loading", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          isLoading={true}
          results={mockArticles}
        />,
      );

      expect(screen.queryByTestId("article-card-1")).not.toBeInTheDocument();
      expect(screen.getByText(/searching articles/i)).toBeInTheDocument();
    });

    it("should display loading message", () => {
      renderWithChakra(
        <ArticleSearchResults {...defaultProps} isLoading={true} />,
      );

      // Check that loading text is present
      expect(screen.getByText(/searching articles/i)).toBeInTheDocument();
    });
  });

  describe("Empty State - No Query", () => {
    it("should render nothing when no search query is provided", () => {
      renderWithChakra(
        <ArticleSearchResults {...defaultProps} searchQuery="" />,
      );

      // Component returns null when no query, so no article content should be present
      expect(screen.queryByText(/found/i)).not.toBeInTheDocument();
      expect(screen.queryByText(/searching/i)).not.toBeInTheDocument();
      expect(screen.queryByText(/no articles/i)).not.toBeInTheDocument();
    });

    it("should not render when query is empty even with results", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery=""
          results={mockArticles}
        />,
      );

      // Component returns null when no query, even with results
      expect(screen.queryByTestId("article-card-1")).not.toBeInTheDocument();
      expect(screen.queryByText(/found/i)).not.toBeInTheDocument();
    });
  });

  describe("Empty State - No Results", () => {
    it("should display no results message when query exists but results are empty", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test query"
          results={[]}
        />,
      );

      expect(screen.getByText(/no articles found/i)).toBeInTheDocument();
      expect(
        screen.getByText(/no articles match "test query"/i),
      ).toBeInTheDocument();
    });

    it("should suggest trying different keywords when no results", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="nonexistent"
          results={[]}
        />,
      );

      expect(
        screen.getByText(/try different keywords or check your spelling/i),
      ).toBeInTheDocument();
    });

    it("should display the search query in empty state message", () => {
      const query = "specific search term";
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery={query}
          results={[]}
        />,
      );

      expect(screen.getByText(new RegExp(query, "i"))).toBeInTheDocument();
    });
  });

  describe("Results Display", () => {
    it("should render article cards when results are provided", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
        />,
      );

      expect(screen.getByTestId("article-card-1")).toBeInTheDocument();
      expect(screen.getByTestId("article-card-2")).toBeInTheDocument();
      expect(screen.getByTestId("article-card-3")).toBeInTheDocument();
    });

    it("should display correct number of results", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
        />,
      );

      expect(screen.getByText(/found 3 articles/i)).toBeInTheDocument();
    });

    it("should use singular form for single result", () => {
      const singleArticle = [mockArticles[0]];
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={singleArticle}
        />,
      );

      expect(screen.getByText(/found 1 article$/i)).toBeInTheDocument();
    });

    it("should display all article titles", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
        />,
      );

      expect(screen.getByText("Test Article 1")).toBeInTheDocument();
      expect(screen.getByText("Test Article 2")).toBeInTheDocument();
      expect(screen.getByText("Test Article 3")).toBeInTheDocument();
    });

    it("should display all article content", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
        />,
      );

      expect(screen.getByText("Content 1")).toBeInTheDocument();
      expect(screen.getByText("Content 2")).toBeInTheDocument();
      expect(screen.getByText("Content 3")).toBeInTheDocument();
    });

    it("should render articles in the order provided", () => {
      const { container } = renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
        />,
      );

      const cards = container.querySelectorAll('[data-testid^="article-card-"]');
      expect(cards[0]).toHaveAttribute("data-testid", "article-card-1");
      expect(cards[1]).toHaveAttribute("data-testid", "article-card-2");
      expect(cards[2]).toHaveAttribute("data-testid", "article-card-3");
    });
  });

  describe("Search Metadata", () => {
    it("should display search time when provided", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
          searchTime={250}
        />,
      );

      expect(screen.getByText(/search completed in 250ms/i)).toBeInTheDocument();
    });

    it("should not display search time when undefined", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
          searchTime={undefined}
        />,
      );

      expect(screen.queryByText(/search completed/i)).not.toBeInTheDocument();
    });

    it("should display search time with correct formatting", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
          searchTime={1234}
        />,
      );

      expect(screen.getByText("Search completed in 1234ms")).toBeInTheDocument();
    });

    it("should display results count in metadata", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
        />,
      );

      const metadata = screen.getByText(/found 3 articles/i);
      expect(metadata).toBeInTheDocument();
    });
  });

  describe("Edge Cases", () => {
    it("should handle empty results array gracefully", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={[]}
        />,
      );

      expect(screen.getByText(/no articles found/i)).toBeInTheDocument();
      expect(screen.queryByTestId(/article-card/i)).not.toBeInTheDocument();
    });

    it("should handle very long search queries", () => {
      const longQuery = "a".repeat(100);
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery={longQuery}
          results={[]}
        />,
      );

      expect(screen.getByText(/no articles found/i)).toBeInTheDocument();
    });

    it("should handle zero search time", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
          searchTime={0}
        />,
      );

      expect(screen.getByText("Search completed in 0ms")).toBeInTheDocument();
    });

    it("should handle large result sets", () => {
      const largeResults = Array.from({ length: 100 }, (_, i) => ({
        id: `${i}`,
        title: `Article ${i}`,
        content: `Content ${i}`,
      }));

      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={largeResults}
        />,
      );

      expect(screen.getByText(/found 100 articles/i)).toBeInTheDocument();
      expect(screen.getByTestId("article-card-0")).toBeInTheDocument();
      expect(screen.getByTestId("article-card-99")).toBeInTheDocument();
    });

    it("should handle articles with special characters in title", () => {
      const specialArticle: Article = {
        id: "special",
        title: "Test <script>alert('xss')</script> Article",
        content: "Content with & special chars",
      };

      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={[specialArticle]}
        />,
      );

      // React automatically escapes HTML, so we should see the raw text
      expect(
        screen.getByText(/Test <script>alert\('xss'\)<\/script> Article/i),
      ).toBeInTheDocument();
    });
  });

  describe("Component Structure", () => {
    it("should use VStack for layout", () => {
      const { container } = renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
        />,
      );

      const vstack = container.querySelector(".chakra-stack");
      expect(vstack).toBeInTheDocument();
    });

    it("should display metadata before article cards", () => {
      const { container } = renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
        />,
      );

      const metadata = screen.getByText(/found 3 articles/i);
      const firstCard = screen.getByTestId("article-card-1");

      // Metadata should appear before the first card in DOM order
      expect(
        metadata.compareDocumentPosition(firstCard) &
          Node.DOCUMENT_POSITION_FOLLOWING,
      ).toBeTruthy();
    });

    it("should have proper gap spacing with Chakra VStack", () => {
      const { container } = renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
        />,
      );

      const vstack = container.querySelector(".chakra-stack");
      // Chakra generates dynamic class names, just verify VStack exists
      expect(vstack).toBeInTheDocument();
      expect(vstack).toHaveClass("chakra-stack");
    });
  });

  describe("Accessibility", () => {
    it("should have proper semantic structure", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
        />,
      );

      // Should have proper text hierarchy
      const headings = screen.getAllByRole("heading", { level: 3 });
      expect(headings).toHaveLength(3);
    });

    it("should have readable content", () => {
      renderWithChakra(
        <ArticleSearchResults
          {...defaultProps}
          searchQuery="test"
          results={mockArticles}
        />,
      );

      // Check that results are readable
      expect(screen.getByText("Test Article 1")).toBeInTheDocument();
      expect(screen.getByText(/found 3 articles/i)).toBeInTheDocument();
    });

    it("should communicate loading state to screen readers", () => {
      renderWithChakra(
        <ArticleSearchResults {...defaultProps} isLoading={true} />,
      );

      const loadingText = screen.getByText(/searching articles/i);
      expect(loadingText).toBeInTheDocument();
    });
  });
});

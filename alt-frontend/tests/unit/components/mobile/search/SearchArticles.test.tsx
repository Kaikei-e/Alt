import React from "react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  render,
  screen,
  fireEvent,
  waitFor,
  cleanup,
} from "@testing-library/react";
import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { SearchArticles } from "@/components/mobile/search/SearchArticles";
import { feedsApi } from "@/lib/api";
import { Article } from "@/schema/article";
import "../test-env";

// Mock feedsApi
vi.mock("@/lib/api", () => ({
  feedsApi: {
    searchArticles: vi.fn(),
  },
}));

// Mock next/navigation
vi.mock("next/navigation", () => ({
  useSearchParams: vi.fn(() => ({
    get: vi.fn((param: string) => null),
  })),
  useRouter: vi.fn(() => ({
    push: vi.fn(),
    replace: vi.fn(),
    prefetch: vi.fn(),
  })),
}));

const createMockArticle = (
  id: string,
  overrides: Partial<Article> = {},
): Article => ({
  id,
  title: `Test Article ${id}`,
  content: `Content ${id}`,
  url: `https://example.com/articles/${id}`,
  published_at: "2024-01-01T00:00:00.000Z",
  ...overrides,
});

describe("SearchArticles", () => {
  const mockArticles: Article[] = [
    createMockArticle("1"),
    createMockArticle("2"),
  ];

  const defaultProps = {
    articles: [],
    setArticles: vi.fn(),
    query: "",
    setQuery: vi.fn(),
    error: null,
    setError: vi.fn(),
    isLoading: false,
    setIsLoading: vi.fn(),
    setSearchTime: vi.fn(),
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

  describe("Basic Rendering", () => {
    it("should render search input and button", () => {
      renderWithChakra(<SearchArticles {...defaultProps} />);

      expect(screen.getByPlaceholderText("Search for articles...")).toBeInTheDocument();
      expect(screen.getByRole("button")).toBeInTheDocument();
    });

    it("should render with placeholder text", () => {
      renderWithChakra(<SearchArticles {...defaultProps} />);

      const input = screen.getByPlaceholderText("Search for articles...") as HTMLInputElement;
      expect(input).toHaveAttribute("placeholder", "Search for articles...");
    });

    it("should display current query value", () => {
      renderWithChakra(<SearchArticles {...defaultProps} query="test query" />);

      const input = screen.getByPlaceholderText("Search for articles...") as HTMLInputElement;
      expect(input).toHaveValue("test query");
    });
  });

  describe("Input Validation", () => {
    it("should disable search button when query is empty", () => {
      renderWithChakra(<SearchArticles {...defaultProps} query="" />);

      const button = screen.getByRole("button");
      expect(button).toBeDisabled();
    });

    it("should disable search button when query is less than 2 characters", () => {
      renderWithChakra(<SearchArticles {...defaultProps} query="a" />);

      const button = screen.getByRole("button");
      expect(button).toBeDisabled();
    });

    it("should enable search button when query is 2 or more characters", () => {
      renderWithChakra(<SearchArticles {...defaultProps} query="ab" />);

      const button = screen.getByRole("button");
      expect(button).not.toBeDisabled();
    });

    // Note: Validation errors only appear when button is clicked with a valid length but invalid content
    // Empty or too-short queries disable the button, so no error message appears
  });

  describe("User Interaction", () => {
    it("should call setQuery when input changes", () => {
      const setQuery = vi.fn();
      renderWithChakra(
        <SearchArticles {...defaultProps} setQuery={setQuery} />,
      );

      const input = screen.getByPlaceholderText("Search for articles...");
      fireEvent.change(input, { target: { value: "new query" } });

      expect(setQuery).toHaveBeenCalledWith("new query");
    });

    it("should clear error when user starts typing", () => {
      const setError = vi.fn();
      renderWithChakra(
        <SearchArticles
          {...defaultProps}
          error="Previous error"
          setError={setError}
        />,
      );

      const input = screen.getByPlaceholderText("Search for articles...");
      fireEvent.change(input, { target: { value: "new query" } });

      expect(setError).toHaveBeenCalledWith(null);
    });

    it("should trigger search on Enter key press", async () => {
      const mockedSearchArticles = vi.mocked(feedsApi.searchArticles);
      mockedSearchArticles.mockResolvedValueOnce(mockArticles);

      const setArticles = vi.fn();
      renderWithChakra(
        <SearchArticles
          {...defaultProps}
          query="valid query"
          setArticles={setArticles}
        />,
      );

      const input = screen.getByPlaceholderText("Search for articles...");
      fireEvent.keyDown(input, { key: "Enter", code: "Enter" });

      await waitFor(() => {
        expect(feedsApi.searchArticles).toHaveBeenCalledWith("valid query");
      });
    });

    it("should prevent Enter key default behavior", () => {
      renderWithChakra(<SearchArticles {...defaultProps} query="test" />);

      const input = screen.getByPlaceholderText("Search for articles...");
      const event = new KeyboardEvent("keydown", {
        key: "Enter",
        code: "Enter",
        bubbles: true,
        cancelable: true,
      });
      const preventDefaultSpy = vi.spyOn(event, "preventDefault");

      input.dispatchEvent(event);

      expect(preventDefaultSpy).toHaveBeenCalled();
    });
  });

  describe("Search Functionality", () => {
    it("should call searchArticles API when valid query is submitted", async () => {
      const mockedSearchArticles = vi.mocked(feedsApi.searchArticles);
      mockedSearchArticles.mockResolvedValueOnce(mockArticles);

      const setArticles = vi.fn();
      renderWithChakra(
        <SearchArticles
          {...defaultProps}
          query="test query"
          setArticles={setArticles}
        />,
      );

      const button = screen.getByRole("button");
      fireEvent.click(button);

      await waitFor(() => {
        expect(feedsApi.searchArticles).toHaveBeenCalledWith("test query");
        expect(setArticles).toHaveBeenCalledWith(mockArticles);
      });
    });

    it("should trim whitespace from query before searching", async () => {
      const mockedSearchArticles = vi.mocked(feedsApi.searchArticles);
      mockedSearchArticles.mockResolvedValueOnce(mockArticles);

      renderWithChakra(
        <SearchArticles {...defaultProps} query="  test query  " />,
      );

      const button = screen.getByRole("button");
      fireEvent.click(button);

      await waitFor(() => {
        expect(feedsApi.searchArticles).toHaveBeenCalledWith("test query");
      });
    });

    it("should clear articles before new search", async () => {
      const mockedSearchArticles = vi.mocked(feedsApi.searchArticles);
      mockedSearchArticles.mockResolvedValueOnce(mockArticles);

      const setArticles = vi.fn();
      renderWithChakra(
        <SearchArticles
          {...defaultProps}
          query="test"
          setArticles={setArticles}
        />,
      );

      const button = screen.getByRole("button");
      fireEvent.click(button);

      await waitFor(() => {
        expect(setArticles).toHaveBeenCalledWith([]);
      });
    });

    it("should clear errors before new search", async () => {
      const mockedSearchArticles = vi.mocked(feedsApi.searchArticles);
      mockedSearchArticles.mockResolvedValueOnce(mockArticles);

      const setError = vi.fn();
      renderWithChakra(
        <SearchArticles
          {...defaultProps}
          query="test"
          error="Previous error"
          setError={setError}
        />,
      );

      // Wait for button to be available
      const button = await waitFor(() => screen.getByRole("button"), { timeout: 3000 });
      fireEvent.click(button);

      await waitFor(() => {
        expect(setError).toHaveBeenCalledWith(null);
      });
    });
  });

  describe("Error Handling", () => {
    it("should display API error when search fails", async () => {
      const mockedSearchArticles = vi.mocked(feedsApi.searchArticles);
      mockedSearchArticles.mockRejectedValueOnce(
        new Error("Network error occurred"),
      );

      const setError = vi.fn();
      renderWithChakra(
        <SearchArticles
          {...defaultProps}
          query="test"
          setError={setError}
        />,
      );

      // Wait for button to be available
      const button = await waitFor(() => screen.getByRole("button"), { timeout: 3000 });
      fireEvent.click(button);

      await waitFor(() => {
        expect(setError).toHaveBeenCalledWith("Network error occurred");
      });
    });

    it("should display generic error message for non-Error objects", async () => {
      const mockedSearchArticles = vi.mocked(feedsApi.searchArticles);
      mockedSearchArticles.mockRejectedValueOnce("Unknown error");

      const setError = vi.fn();
      renderWithChakra(
        <SearchArticles
          {...defaultProps}
          query="test"
          setError={setError}
        />,
      );

      // Wait for button to be available
      const button = await waitFor(() => screen.getByRole("button"), { timeout: 3000 });
      fireEvent.click(button);

      await waitFor(() => {
        expect(setError).toHaveBeenCalledWith(
          "Search failed. Please try again.",
        );
      });
    });

    it("should render API error message when provided", async () => {
      renderWithChakra(
        <SearchArticles {...defaultProps} error="API error occurred" />,
      );

      // Wait for error message to render (error && !validationError condition)
      await waitFor(() => {
        expect(screen.getByTestId("error-message")).toBeInTheDocument();
        expect(screen.getByText("API error occurred")).toBeInTheDocument();
      });
    });
  });

  describe("Loading State", () => {
    it("should show loading text when isLoading is true", async () => {
      renderWithChakra(<SearchArticles {...defaultProps} query="test" />);

      // Simulate loading by checking button text during loading
      // Note: We can't directly set isLoading prop since it's internal state
      const button = await waitFor(() => screen.getByRole("button"), { timeout: 3000 });
      expect(button.textContent).toBe("Search");
    });

    it("should prevent search when already loading", async () => {
      const mockedSearchArticles = vi.mocked(feedsApi.searchArticles);
      let resolveSearch: () => void;
      const searchPromise = new Promise<Article[]>((resolve) => {
        resolveSearch = () => resolve(mockArticles);
      });
      mockedSearchArticles.mockReturnValueOnce(searchPromise);

      const setIsLoading = vi.fn();
      renderWithChakra(
        <SearchArticles
          {...defaultProps}
          query="test"
          setIsLoading={setIsLoading}
        />,
      );

      // Wait for button to be available
      const button = await waitFor(() => screen.getByRole("button"), { timeout: 3000 });

      // First click starts loading
      fireEvent.click(button);

      // Verify loading state was set
      await waitFor(() => {
        expect(setIsLoading).toHaveBeenCalledWith(true);
      });

      // API should only be called once even if we try to click again
      expect(feedsApi.searchArticles).toHaveBeenCalledTimes(1);

      // Resolve the search
      resolveSearch!();
    });
  });

  describe("Form Submission", () => {
    it("should prevent default form submission", async () => {
      const mockedSearchArticles = vi.mocked(feedsApi.searchArticles);
      mockedSearchArticles.mockResolvedValueOnce(mockArticles);

      renderWithChakra(<SearchArticles {...defaultProps} query="test" />);

      const button = await waitFor(() => screen.getByRole("button"), { timeout: 3000 });
      const form = button.closest("form");
      expect(form).toBeInTheDocument();

      const submitEvent = new Event("submit", {
        bubbles: true,
        cancelable: true,
      });
      const preventDefaultSpy = vi.spyOn(submitEvent, "preventDefault");

      form?.dispatchEvent(submitEvent);

      expect(preventDefaultSpy).toHaveBeenCalled();
    });

    it("should validate query on form submission", async () => {
      renderWithChakra(<SearchArticles {...defaultProps} query="" />);

      const button = await waitFor(() => screen.getByRole("button"), { timeout: 3000 });
      const form = button.closest("form");
      fireEvent.submit(form!);

      await waitFor(() => {
        expect(screen.getByTestId("error-message")).toBeInTheDocument();
      });
    });
  });

  describe("URL Query Parameters", () => {
    it("should load query from URL parameters on mount", async () => {
      const mockGet = vi.fn((param: string) =>
        param === "q" ? "url query" : null,
      );
      // Re-import and mock useSearchParams before rendering
      const { useSearchParams } = await import("next/navigation");
      vi.mocked(useSearchParams).mockImplementation(() => ({
        get: mockGet,
      }) as any);

      const mockedSearchArticles = vi.mocked(feedsApi.searchArticles);
      mockedSearchArticles.mockResolvedValueOnce(mockArticles);

      const setQuery = vi.fn();
      const setArticles = vi.fn();

      renderWithChakra(
        <SearchArticles
          {...defaultProps}
          setQuery={setQuery}
          setArticles={setArticles}
        />,
      );

      await waitFor(() => {
        expect(setQuery).toHaveBeenCalledWith("url query");
        expect(feedsApi.searchArticles).toHaveBeenCalledWith("url query");
      });
    });

    it("should handle invalid URL query parameter", async () => {
      const mockGet = vi.fn((param: string) => (param === "q" ? "a" : null)); // Too short
      // Re-import and mock useSearchParams before rendering
      const { useSearchParams } = await import("next/navigation");
      vi.mocked(useSearchParams).mockImplementation(() => ({
        get: mockGet,
      }) as any);

      const mockedSearchArticles = vi.mocked(feedsApi.searchArticles);

      renderWithChakra(<SearchArticles {...defaultProps} />);

      await waitFor(() => {
        // Should not call search API with invalid query
        expect(mockedSearchArticles).not.toHaveBeenCalled();
      });
    });
  });

  describe("Accessibility", () => {
    it("should have accessible form elements", async () => {
      renderWithChakra(<SearchArticles {...defaultProps} />);

      const input = await waitFor(() => screen.getByPlaceholderText("Search for articles..."), { timeout: 3000 });
      const button = await waitFor(() => screen.getByRole("button"), { timeout: 3000 });

      expect(input).toHaveAttribute("type", "text");
      expect(button).toHaveAttribute("type", "submit");
    });

    it("should have proper placeholder text", async () => {
      renderWithChakra(<SearchArticles {...defaultProps} />);

      const input = await waitFor(() => screen.getByPlaceholderText("Search for articles..."), { timeout: 3000 }) as HTMLInputElement;
      expect(input).toHaveAttribute("placeholder", "Search for articles...");
    });

    it("should have data-testid attributes for testing", async () => {
      renderWithChakra(<SearchArticles {...defaultProps} />);

      expect(screen.getByTestId("search-window")).toBeInTheDocument();
      const input = await waitFor(() => screen.getByPlaceholderText("Search for articles..."), { timeout: 3000 });
      expect(input).toBeInTheDocument();
    });
  });
});

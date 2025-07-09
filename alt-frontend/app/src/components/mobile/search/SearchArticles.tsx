"use client";

import { useEffect, useState } from "react";
import { Article } from "@/schema/article";
import { feedsApi } from "@/lib/api";
import { Input, Box, VStack, Text } from "@chakra-ui/react";
import { articleSearchQuerySchema } from "@/schema/validation/articleSearchQuery";
import * as v from "valibot";
import { useSearchParams } from "next/navigation";
// import { escapeForDisplay } from "@/utils/htmlEscape";

interface SearchArticlesProps {
  articles: Article[];
  setArticles: (articles: Article[]) => void;
  query: string;
  setQuery: (query: string) => void;
  error: string | null;
  setError: (error: string | null) => void;
}

export const SearchArticles = ({
  setArticles,
  query,
  setQuery,
  error,
  setError,
}: SearchArticlesProps) => {
  const [isLoading, setIsLoading] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);
  const searchParams = useSearchParams();

  // Handle URL query parameter on mount
  useEffect(() => {
    if (!searchParams) return;

    const urlQuery = searchParams.get("q");
    if (urlQuery) {
      try {
        // URLパラメータを厳密に検証
        const result = v.safeParse(articleSearchQuerySchema, { query: urlQuery });
        if (result.success) {
          // 検証に成功した場合のみ、クエリを設定
          setQuery(urlQuery);
          setError(null);
          setValidationError(null);
          setIsLoading(true);
          feedsApi
            .searchArticles(urlQuery)
            .then((response) => {
              setArticles(response);
              setIsLoading(false);
            })
            .catch((err) => {
              setError(err.message || "Search failed. Please try again.");
              setIsLoading(false);
            });
        } else {
          // 無効なURLクエリの場合は警告を記録し、無視
          console.warn('Invalid URL query parameter detected:', urlQuery);
          setValidationError("Invalid search query from URL");
        }
      } catch (error) {
        console.warn('Error processing URL query parameter:', error);
        setValidationError("Invalid search query from URL");
      }
    }
  }, [searchParams, setQuery, setError, setArticles]);

  // Validate query function
  const validateQuery = (queryText: string) => {
    const trimmed = queryText.trim();

    if (!trimmed) {
      return "Please enter a search query";
    }

    if (trimmed.length < 2) {
      return "Search query must be at least 2 characters";
    }

    const result = v.safeParse(articleSearchQuerySchema, { query: trimmed });
    if (!result.success) {
      return result.issues?.[0]?.message || "Please enter a valid search query";
    }

    return null;
  };

  const handleSearch = async () => {
    if (isLoading) return;

    const validationResult = validateQuery(query);

    if (validationResult) {
      setValidationError(validationResult);
      return;
    }

    setError(null);
    setValidationError(null);
    setIsLoading(true);

    try {
      setArticles([]);
      const response = await feedsApi.searchArticles(query.trim());
      setArticles(response);
    } catch (err) {
      console.error("Search error:", err);
      setError(
        err instanceof Error ? err.message : "Search failed. Please try again.",
      );
    } finally {
      setIsLoading(false);
    }
  };

  const handleFormSubmit = (e: React.FormEvent) => {
    e.preventDefault();

    // Always validate on form submit, regardless of button state
    const validationResult = validateQuery(query);

    if (validationResult) {
      setValidationError(validationResult);
      return;
    }

    // Only proceed with search if validation passes
    handleSearch();
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault();
      // Trigger form submission which will handle validation
      const form = e.currentTarget.closest("form");
      if (form) {
        const submitEvent = new Event("submit", {
          bubbles: true,
          cancelable: true,
        });
        form.dispatchEvent(submitEvent);
      }
    }
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const newQuery = e.target.value;
    setQuery(newQuery);

    // Clear API errors when user starts typing
    if (error) setError(null);

    // Clear validation error when user types enough characters
    if (newQuery.trim().length >= 2) {
      setValidationError(null);
    }
  };

  // Enhanced button click handler that always validates
  const handleButtonClick = () => {
    // Always validate when button is clicked
    const validationResult = validateQuery(query);
    if (validationResult) {
      setValidationError(validationResult);
      // Don't prevent default - let form submission handle it
      return;
    }
  };

  // Button disabled logic
  const isButtonDisabled = isLoading || query.trim().length < 2;

  return (
    <Box width="100%" maxWidth="500px" mb={6} data-testid="search-window">
      <form onSubmit={handleFormSubmit}>
        <VStack gap={4}>
          <Box width="full">
            <Text
              color="var(--alt-primary)"
              mb={2}
              fontSize="2xl"
              fontWeight="bold"
              textAlign="center"
            >
              Search Articles
            </Text>
            <Input
              type="text"
              placeholder="Search for articles..."
              textAlign="center"
              value={query}
              onChange={handleInputChange}
              onKeyDown={handleKeyPress}
              bg="rgba(255, 255, 255, 0.1)"
              border={`1px solid ${validationError ? "#f87171" : "rgba(255, 255, 255, 0.2)"}`}
              color="white"
              _placeholder={{ color: "rgba(255, 255, 255, 0.5)" }}
              _focus={{
                borderColor: validationError ? "#f87171" : "#ff006e",
                boxShadow: `0 0 0 1px ${validationError ? "#f87171" : "#ff006e"}`,
              }}
              borderRadius="15px"
              p={4}
              data-testid="search-input"
            />
          </Box>

          {/* Use native button for more control */}
          <button
            type="submit"
            disabled={isButtonDisabled}
            onClick={handleButtonClick}
            style={{
              background: isButtonDisabled
                ? "rgba(255, 255, 255, 0.3)"
                : "linear-gradient(45deg, #ff006e, #8338ec)",
              color: "white",
              fontWeight: "bold",
              padding: "16px 32px",
              borderRadius: "9999px",
              border: "1px solid rgba(255, 255, 255, 0.2)",
              width: "100%",
              cursor: isButtonDisabled ? "not-allowed" : "pointer",
              opacity: isButtonDisabled ? 0.6 : 1,
              transition: "all 0.2s ease",
              fontSize: "16px",
            }}
            onMouseEnter={(e) => {
              if (!isButtonDisabled) {
                e.currentTarget.style.background =
                  "linear-gradient(45deg, #e6005c, #7129d4)";
                e.currentTarget.style.transform = "translateY(-2px)";
              }
            }}
            onMouseLeave={(e) => {
              if (!isButtonDisabled) {
                e.currentTarget.style.background =
                  "linear-gradient(45deg, #ff006e, #8338ec)";
                e.currentTarget.style.transform = "translateY(0px)";
              }
            }}
          >
            {isLoading ? "Searching..." : "Search"}
          </button>

          {/* Always show validation error when present */}
          {validationError && (
            <Text
              color="#f87171"
              textAlign="center"
              fontSize="sm"
              fontWeight="medium"
              data-testid="error-message"
            >
              {validationError}
            </Text>
          )}

          {/* Show API error when present */}
          {error && !validationError && (
            <Text
              color="#f87171"
              textAlign="center"
              fontSize="sm"
              fontWeight="medium"
              data-testid="error-message"
            >
              {error}
            </Text>
          )}
        </VStack>
      </form>
    </Box>
  );
};

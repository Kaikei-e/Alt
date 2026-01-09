"use client";

import { Box, Input, Text, VStack } from "@chakra-ui/react";
import { useSearchParams } from "next/navigation";
import { useEffect, useState } from "react";
import * as v from "valibot";
import { articleApi } from "@/lib/api";
import type { Article } from "@/schema/article";
import { articleSearchQuerySchema } from "@/schema/validation/articleSearchQuery";

interface SearchArticlesProps {
  articles: Article[];
  setArticles: (articles: Article[]) => void;
  query: string;
  setQuery: (query: string) => void;
  error: string | null;
  setError: (error: string | null) => void;
  isLoading: boolean;
  setIsLoading: (isLoading: boolean) => void;
  setSearchTime: (searchTime: number) => void;
}

export const SearchArticles = ({
  setArticles,
  query,
  setQuery,
  error,
  setError,
  isLoading,
  setIsLoading,
  setSearchTime,
}: SearchArticlesProps) => {
  const [validationError, setValidationError] = useState<string | null>(null);
  const searchParams = useSearchParams();

  // Handle URL query parameter on mount
  useEffect(() => {
    if (!searchParams) return;

    const urlQuery = searchParams.get("q");
    if (urlQuery) {
      try {
        // URLパラメータを厳密に検証
        const result = v.safeParse(articleSearchQuerySchema, {
          query: urlQuery,
        });
        if (result.success) {
          // 検証に成功した場合のみ、クエリを設定
          setQuery(urlQuery);
          setError(null);
          setValidationError(null);
          setIsLoading(true);
          articleApi
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
          setValidationError("Invalid search query from URL");
        }
      } catch (error) {
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
      const response = await articleApi.searchArticles(query.trim());
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
      // Directly call the form submission logic
      const validationResult = validateQuery(query);

      if (validationResult) {
        setValidationError(validationResult);
        return;
      }

      // Only proceed with search if validation passes
      handleSearch();
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
  const handleButtonClick = (e: React.MouseEvent) => {
    // Always validate when button is clicked
    const validationResult = validateQuery(query);
    if (validationResult) {
      e.preventDefault();
      setValidationError(validationResult);
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
            <Input
              type="text"
              placeholder="Search for articles..."
              textAlign="center"
              value={query}
              onChange={handleInputChange}
              onKeyDown={handleKeyPress}
              bg="var(--surface-bg)"
              border={`2px solid ${validationError ? "#dc2626" : "var(--surface-border)"}`}
              color="var(--text-primary)"
              disabled={isLoading}
              _placeholder={{ color: "var(--text-muted)" }}
              _focus={{
                borderColor: validationError ? "#dc2626" : "var(--alt-primary)",
                boxShadow: "var(--shadow-sm)",
                outline: "none",
              }}
              _hover={{
                borderColor: validationError
                  ? "#dc2626"
                  : "var(--alt-secondary)",
              }}
              _disabled={{
                bg: "var(--surface-bg)",
                borderColor: "var(--surface-border)",
                color: "var(--text-muted)",
                opacity: 0.6,
                cursor: "not-allowed",
              }}
              borderRadius="12px"
              p={4}
              data-testid="search-input"
            />
          </Box>

          {/* Use native button for more control */}
          <button
            type="submit"
            disabled={isButtonDisabled}
            onClick={handleButtonClick}
            data-testid="search-button"
            style={{
              background: isButtonDisabled
                ? "var(--surface-bg)"
                : "var(--surface-bg)",
              color: isButtonDisabled
                ? "var(--text-muted)"
                : "var(--text-primary)",
              fontWeight: "700",
              padding: "16px 32px",
              borderRadius: "0",
              border: `2px solid ${isButtonDisabled ? "var(--surface-border)" : "var(--alt-primary)"}`,
              width: "100%",
              cursor: isButtonDisabled ? "not-allowed" : "pointer",
              opacity: isButtonDisabled ? 0.6 : 1,
              transition: "all 0.2s ease",
              fontSize: "16px",
              letterSpacing: "0.025em",
            }}
            onMouseEnter={(e) => {
              if (!isButtonDisabled) {
                e.currentTarget.style.background = "var(--alt-primary)";
                e.currentTarget.style.color = "#ffffff";
                e.currentTarget.style.boxShadow = "var(--shadow-md)";
              }
            }}
            onMouseLeave={(e) => {
              if (!isButtonDisabled) {
                e.currentTarget.style.background = "var(--surface-bg)";
                e.currentTarget.style.color = "var(--text-primary)";
                e.currentTarget.style.boxShadow = "none";
              }
            }}
          >
            {isLoading ? "Searching..." : "Search"}
          </button>

          {/* Always show validation error when present */}
          {validationError && (
            <Text
              color="#dc2626"
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
              color="#dc2626"
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

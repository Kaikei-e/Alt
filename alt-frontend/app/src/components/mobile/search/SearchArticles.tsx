"use client";

import { useEffect, useState } from "react";
import { Article } from "@/schema/article";
import { feedsApi } from "@/lib/api";
import { Button, Input, Box, VStack, Text } from "@chakra-ui/react";
import { articleSearchQuerySchema } from "@/schema/validation/articleSearchQuery";
import * as v from "valibot";
import { useSearchParams } from "next/navigation";

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
  setError
}: SearchArticlesProps) => {
  const [isLoading, setIsLoading] = useState(false);
  const [validationError, setValidationError] = useState<string | null>(null);
  const searchParams = useSearchParams();

  // Handle URL query parameter on mount
  useEffect(() => {
    const urlQuery = searchParams.get('q');
    if (urlQuery) {
      setQuery(urlQuery);
      // Auto-search if there's a query in the URL
      const result = v.safeParse(articleSearchQuerySchema, { query: urlQuery });
      if (result.success) {
        setError(null);
        setIsLoading(true);
        feedsApi.searchArticles(urlQuery).then((response) => {
          setArticles(response);
          setIsLoading(false);
        }).catch((err) => {
          setError(err.message || "Search failed. Please try again.");
          setIsLoading(false);
        });
      }
    }
  }, [searchParams, setQuery, setError, setArticles]);

  // Real-time validation
  const validateQuery = (queryText: string) => {
    const validationResult = v.safeParse(articleSearchQuerySchema, { query: queryText });
    if (!validationResult.success) {
      const firstError = validationResult.issues?.[0]?.message || "Please enter a valid search query";
      setValidationError(firstError);
      return false;
    }
    setValidationError(null);
    return true;
  };

  const handleSearch = async () => {
    if (isLoading) return;

    setIsLoading(true);
    setError(null);
    setValidationError(null);

    try {
      // Clear previous results
      setArticles([]);

      // Validate input
      const result = v.safeParse(articleSearchQuerySchema, { query });
      if (!result.success) {
        const firstError = result.issues?.[0]?.message || "Please enter a valid search query";
        setValidationError(firstError);
        return;
      }

      // Call API
      const response = await feedsApi.searchArticles(query);
      setArticles(response);
    } catch (err) {
      console.error("Search error:", err);
      setError(err instanceof Error ? err.message : "Search failed. Please try again.");
    } finally {
      setIsLoading(false);
    }
  };

  const handleFormSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    handleSearch();
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleSearch();
    }
  };

  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const newQuery = e.target.value;
    setQuery(newQuery);

    // Clear errors when user starts typing
    if (error) setError(null);

    // Real-time validation for better UX
    if (newQuery.trim()) {
      validateQuery(newQuery);
    } else {
      setValidationError(null);
    }
  };

  return (
    <Box width="100%" maxWidth="500px" mb={6}>
      <form onSubmit={handleFormSubmit}>
        <VStack gap={4}>
          <Box width="full">
            <Text
              color="rgba(255, 255, 255, 0.9)"
              mb={2}
              fontSize="sm"
              fontWeight="medium"
            >
              Search Articles
            </Text>
            <Input
              type="text"
              placeholder="Search for articles..."
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
              py={6}
            />
          </Box>

          <Button
            type="submit"
            loading={isLoading}
            bg="linear-gradient(45deg, #ff006e, #8338ec)"
            color="white"
            fontWeight="bold"
            px={8}
            py={6}
            borderRadius="full"
            _hover={{
              bg: "linear-gradient(45deg, #e6005c, #7129d4)",
              transform: "translateY(-2px)",
            }}
            _active={{
              transform: "translateY(0px)",
            }}
            transition="all 0.2s ease"
            border="1px solid rgba(255, 255, 255, 0.2)"
            width="full"
            disabled={isLoading || !!validationError}
            opacity={validationError ? 0.6 : 1}
          >
            {isLoading ? "Searching..." : "Search"}
          </Button>

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

          {error && (
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
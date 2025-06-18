import { useState } from "react";
import { Box, Button, Input, Text, VStack } from "@chakra-ui/react";
import {
  SearchQuery,
  searchQuerySchema,
} from "@/schema/validation/searchQuery";
import { feedsApi } from "@/lib/api";
import type { BackendFeedItem } from "@/schema/feed";
import { transformFeedSearchResult } from "@/lib/utils/transformFeedSearchResult";
import * as v from "valibot";

interface SearchWindowProps {
  searchQuery: SearchQuery;
  setSearchQuery: (query: SearchQuery) => void;
  feedResults: BackendFeedItem[];
  setFeedResults: (results: BackendFeedItem[]) => void;
  isLoading: boolean;
  setIsLoading: (loading: boolean) => void;
  setSearchTime?: (time: number) => void;
}

const SearchWindow = ({
  searchQuery,
  setSearchQuery,
  setFeedResults,
  isLoading,
  setIsLoading,
  setSearchTime,
}: SearchWindowProps) => {
  const [error, setError] = useState<string | null>(null);
  const [validationError, setValidationError] = useState<string | null>(null);

  // Real-time validation
  const validateQuery = (query: SearchQuery) => {
    const validationResult = v.safeParse(searchQuerySchema, query);
    if (!validationResult.success) {
      const firstError =
        validationResult.issues?.[0]?.message ||
        "Please enter a valid search query";
      setValidationError(firstError);
      return false;
    }
    setValidationError(null);
    return true;
  };

  const handleSearch = async () => {
    if (isLoading) return;

    const startTime = Date.now();
    setIsLoading(true);
    setError(null);
    setValidationError(null);

    try {
      // 1. Clear previous results
      setFeedResults([]);

      // 2. Validate input
      const validationResult = v.safeParse(searchQuerySchema, searchQuery);

      if (!validationResult.success) {
        const firstError =
          validationResult.issues?.[0]?.message ||
          "Please enter a valid search query";
        setValidationError(firstError);
        return;
      }

      const validatedQuery = validationResult.output.query;

      // 3. Call API
      const results = await feedsApi.searchFeeds(validatedQuery);

      if (results.error) {
        setError(results.error);
        return;
      }

      // 4. Transform results
      const transformedResults = transformFeedSearchResult(results);

      // 5. Pass results to parent via prop function
      setFeedResults(transformedResults);

      // 6. Track search time
      const searchTime = Date.now() - startTime;
      setSearchTime?.(searchTime);
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
    handleSearch();
  };

  const handleKeyPress = (e: React.KeyboardEvent) => {
    if (e.key === "Enter") {
      e.preventDefault();
      handleSearch();
    }
  };

  return (
    <Box data-testid="search-window">
      <form onSubmit={handleFormSubmit}>
        <VStack gap={4}>
          <Box width="full">
            <Text
              color="rgba(255, 255, 255, 0.9)"
              mb={2}
              fontSize="sm"
              fontWeight="medium"
            >
              Search Query
            </Text>
            <Input
              data-testid="search-input"
              type="text"
              placeholder="Search for feeds..."
              value={searchQuery.query || ""}
              onChange={(e: React.ChangeEvent<HTMLInputElement>) => {
                const newQuery = { query: e.target.value };
                setSearchQuery(newQuery);
                // Clear errors when user starts typing
                if (error) setError(null);
                // Real-time validation for better UX
                if (e.target.value.trim()) {
                  validateQuery(newQuery);
                } else {
                  setValidationError(null);
                }
              }}
              onKeyDown={handleKeyPress}
              bg="rgba(255, 255, 255, 0.1)"
              border={`1px solid ${validationError ? "#f87171" : "rgba(255, 255, 255, 0.2)"}`}
              color="white"
              _placeholder={{ color: "rgba(255, 255, 255, 0.5)" }}
              _focus={{
                borderColor: validationError ? "#f87171" : "#ff006e",
                boxShadow: `0 0 0 1px ${validationError ? "#f87171" : "#ff006e"}`,
              }}
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
            >
              {error}
            </Text>
          )}
        </VStack>
      </form>
    </Box>
  );
};

export default SearchWindow;

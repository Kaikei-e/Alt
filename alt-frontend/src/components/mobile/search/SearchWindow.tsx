import { Box, Button, Flex, Input, Spinner, Text, VStack } from "@chakra-ui/react";
import { useState, useRef, useTransition, useEffect } from "react";
import * as v from "valibot";
import { feedApi } from "@/lib/api";
import { transformFeedSearchResult } from "@/lib/utils/transformFeedSearchResult";
import type { BackendFeedItem } from "@/schema/feed";
import {
  type SearchQuery,
  searchQuerySchema,
} from "@/schema/validation/searchQuery";

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
  const [isPending, startTransition] = useTransition();
  const abortControllerRef = useRef<AbortController | null>(null);

  // Cancel previous request when query changes
  useEffect(() => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }
  }, [searchQuery.query]);

  // Validate query function
  const validateQuery = (queryText: string) => {
    const trimmed = queryText.trim();

    if (!trimmed) {
      return "Please enter a search query";
    }

    if (trimmed.length < 2) {
      return "Search query must be at least 2 characters";
    }

    const result = v.safeParse(searchQuerySchema, { query: trimmed });
    if (!result.success) {
      return result.issues?.[0]?.message || "Please enter a valid search query";
    }

    return null;
  };

  const handleSearch = async () => {
    if (isLoading || isPending) return;

    // Cancel any in-flight request
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
    }

    const abortController = new AbortController();
    abortControllerRef.current = abortController;

    const startTime = Date.now();
    setIsLoading(true);
    setError(null);
    setValidationError(null);

    try {
      // 1. Clear previous results
      setFeedResults([]);

      // 2. Validate input
      const validationResult = validateQuery(searchQuery.query || "");

      if (validationResult) {
        setValidationError(validationResult);
        setIsLoading(false);
        return;
      }

      const validatedQuery = searchQuery.query.trim();

      // 3. Call API with abort signal
      // Note: feedApi.searchFeeds needs to support AbortSignal
      // For now, we'll wrap it in a promise that can be cancelled
      const searchPromise = feedApi.searchFeeds(validatedQuery);

      // Check if aborted before proceeding
      if (abortController.signal.aborted) {
        return;
      }

      const results = await searchPromise;

      // Check again after await
      if (abortController.signal.aborted) {
        return;
      }

      if (results.error) {
        setError(results.error);
        return;
      }

      // 4. Transform results
      const transformedResults = transformFeedSearchResult(results);

      // 5. Pass results to parent via transition
      startTransition(() => {
        setFeedResults(transformedResults);
      });

      // 6. Track search time
      const searchTime = Date.now() - startTime;
      setSearchTime?.(searchTime);
    } catch (err) {
      if (abortController.signal.aborted) {
        return; // Ignore abort errors
      }
      console.error("Search error:", err);
      setError(
        err instanceof Error ? err.message : "Search failed. Please try again.",
      );
    } finally {
      if (!abortController.signal.aborted) {
        setIsLoading(false);
      }
    }
  };

  const handleFormSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    // Always validate on form submit
    const validationResult = validateQuery(searchQuery.query || "");
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
      const validationResult = validateQuery(searchQuery.query || "");

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
    setSearchQuery({ query: newQuery });

    // Clear API errors when user starts typing
    if (error) setError(null);

    // Clear validation error when user types enough characters
    if (newQuery.trim().length >= 2) {
      setValidationError(null);
    }
  };

  return (
    <Box data-testid="search-window">
      <form onSubmit={handleFormSubmit}>
        <VStack gap={4}>
          <Box width="full">
            <Text
              color="var(--text-secondary)"
              mb={2}
              fontSize="sm"
              fontWeight="medium"
            >
              Search Query
            </Text>
            <Input
              data-testid="search-input"
              type="text"
              value={searchQuery.query || ""}
              onChange={handleInputChange}
              placeholder="e.g. AI, technology, startup..."
              bg="var(--bg-surface)"
              border={`1px solid ${validationError ? "#dc2626" : "var(--border-glass)"}`}
              color="var(--text-primary)"
              borderRadius="12px"
              height="48px"
              disabled={isLoading}
              _placeholder={{ color: "var(--text-muted)" }}
              _focus={{
                borderColor: validationError ? "#dc2626" : "var(--alt-primary)",
                boxShadow: "0 0 0 2px var(--alt-primary-alpha)",
                outline: "none",
              }}
              _hover={{
                borderColor: validationError
                  ? "#dc2626"
                  : "var(--alt-secondary)",
              }}
              _disabled={{
                bg: "var(--bg-surface)",
                borderColor: "var(--border-glass)",
                color: "var(--text-muted)",
                opacity: 0.6,
                cursor: "not-allowed",
              }}
              onKeyDown={handleKeyPress}
            />
          </Box>

          <Button
            type="submit"
            bg="var(--alt-primary)"
            color="white"
            fontWeight="600"
            px={8}
            py={6}
            borderRadius="full"
            border="none"
            _hover={{
              bg: "var(--alt-primary-hover)",
              transform: "translateY(-1px)",
              boxShadow: "0 4px 12px var(--alt-primary-shadow)",
            }}
            _active={{
              transform: "translateY(0)",
            }}
            transition="all 0.2s ease"
            width="full"
            disabled={isLoading || isPending || !!validationError}
            opacity={validationError ? 0.6 : 1}
            letterSpacing="0.025em"
          >
            {isLoading || isPending ? (
              <Flex align="center" gap={2}>
                <Spinner size="sm" color="white" />
                Searching...
              </Flex>
            ) : (
              "Search"
            )}
          </Button>

          {validationError && (
            <Text
              color="#dc2626"
              textAlign="center"
              fontSize="sm"
              fontWeight="medium"
            >
              {validationError}
            </Text>
          )}

          {error && (
            <Text
              color="#dc2626"
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
